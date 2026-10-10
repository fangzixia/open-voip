package media

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"open-switch/internal/ports/dto"
)

// Real Pion ICE/DTLS/SRTP and UDP transport, with deterministic G.711 sources.
// Physical phone/browser capture and carrier delay still need endpoint QA.
func TestSIPWebRTCUnifiedBidirectionalRecording(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, e := NewService(Options{AudioRecDir: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.CreateRoom(ctx, "quality", dto.RoomOptions{SessionType: dto.SessionTypeAudio}); e != nil {
		t.Fatal(e)
	}
	defer s.CloseRoom(context.Background(), "quality")
	serverUDP, telephone := udpSocket(t), udpSocket(t)
	rt := &sipRTP{conn: serverUDP, remote: telephone.LocalAddr().(*net.UDPAddr), callID: "quality", legID: "customer", remotePT: 8, remoteCodec: sipCodecPCMA}
	s.attachSIPRTP("quality", rt)
	go s.sipReadLoop("quality", rt)
	browser, e := s.apiPCMU.NewPeerConnection(webrtc.Configuration{})
	if e != nil {
		t.Fatal(e)
	}
	defer browser.Close()
	uplink, e := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000}, "agent", "quality-agent")
	if e != nil {
		t.Fatal(e)
	}
	sender, e := browser.AddTrack(uplink)
	if e != nil {
		t.Fatal(e)
	}
	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, e := sender.Read(buf); e != nil {
				return
			}
		}
	}()
	browserFrames, phoneFrames := make(chan *rtp.Packet, 256), make(chan *rtp.Packet, 256)
	browser.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		go func() {
			for {
				p, _, e := track.ReadRTP()
				if e != nil {
					return
				}
				select {
				case browserFrames <- p:
				default:
				}
			}
		}()
	})
	go func() {
		buf := make([]byte, 1500)
		for {
			n, _, e := telephone.ReadFromUDP(buf)
			if e != nil {
				return
			}
			p := &rtp.Packet{}
			if p.Unmarshal(buf[:n]) == nil {
				select {
				case phoneFrames <- p.Clone():
				default:
				}
			}
		}
	}()
	offer, e := s.JoinWebRTC(ctx, "quality", "agent", dto.LegRoleAgent)
	if e != nil {
		t.Fatal(e)
	}
	if e = browser.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer.SDP}); e != nil {
		t.Fatal(e)
	}
	answer, e := browser.CreateAnswer(nil)
	if e != nil {
		t.Fatal(e)
	}
	gather := webrtc.GatheringCompletePromise(browser)
	if e = browser.SetLocalDescription(answer); e != nil {
		t.Fatal(e)
	}
	select {
	case <-gather:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if e = s.AcceptAnswer(ctx, "quality", "agent", browser.LocalDescription().SDP); e != nil {
		t.Fatal(e)
	}
	for browser.ConnectionState() != webrtc.PeerConnectionStateConnected {
		select {
		case <-ctx.Done():
			t.Fatal("Pion transport did not connect")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if e = s.BridgeLegs(ctx, "quality", "customer", "agent"); e != nil {
		t.Fatal(e)
	}
	recID, e := s.StartRecording(ctx, "quality", dto.RecordingPolicy{Mode: "audio"})
	if e != nil {
		t.Fatal(e)
	}
	clock := time.NewTicker(20 * time.Millisecond)
	defer clock.Stop()
	for i := range 50 {
		<-clock.C
		p := &rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 8, SSRC: 11, SequenceNumber: uint16(i), Timestamp: uint32(i * 160)}, Payload: pcmToG711(constantPCM(4000), 8)}
		raw, _ := p.Marshal()
		if _, e = telephone.WriteToUDP(raw, serverUDP.LocalAddr().(*net.UDPAddr)); e != nil {
			t.Fatal(e)
		}
		p.PayloadType, p.SSRC, p.Payload = 0, 12, pcmToPCMU(constantPCM(7000))
		if e = uplink.WriteRTP(p); e != nil {
			t.Fatal(e)
		}
	}
	time.Sleep(100 * time.Millisecond)
	check := func(ch chan *rtp.Packet, pt uint8, low, high int16) {
		voiced, count := 0, 0
		minSample, maxSample := int16(32767), int16(-32768)
		var previous *rtp.Packet
		for len(ch) > 0 {
			p := <-ch
			if p.PayloadType != pt || len(p.Payload) != 160 {
				t.Fatal("negotiated codec/output framing mismatch")
			}
			if previous != nil && (p.SSRC != previous.SSRC || p.SequenceNumber != previous.SequenceNumber+1 || p.Timestamp-previous.Timestamp != 160) {
				t.Fatal("duplicate producer or discontinuous RTP output")
			}
			previous = p
			v := pcmuPayloadToPCM(rtpPayloadToPCMU(pt, p.Payload))[159]
			count++
			minSample, maxSample = min(minSample, v), max(maxSample, v)
			if v >= low && v <= high {
				voiced++
			}
		}
		if voiced < 30 {
			t.Fatalf("missing other party or self echo: pt=%d voiced=%d count=%d range=%d..%d quality=%+v", pt, voiced, count, minSample, maxSample, s.getRoom("quality").mixer.quality(s.getRoom("quality").rec))
		}
	}
	check(phoneFrames, 8, 5600, 6400)
	check(browserFrames, 0, 3100, 3800)
	if e = s.StopRecording(ctx, recID); e != nil {
		t.Fatal(e)
	}
	meta, e := s.RecordingInfo(ctx, recID)
	if e != nil || meta.Status != "completed" || len(meta.LegPaths) != 2 {
		t.Fatalf("recording incomplete: %+v %v", meta, e)
	}
	main := readRecording(t, meta.FilePath)
	for _, path := range meta.LegPaths {
		if len(readRecording(t, path)) != len(main) {
			t.Fatal("transport stems misaligned")
		}
	}
	if len(main) < 8000 || main[4000] < 4300 || main[4000] > 4900 {
		t.Fatal("main recording omitted or duplicated a party")
	}
}
