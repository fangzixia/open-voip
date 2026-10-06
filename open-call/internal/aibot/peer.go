package aibot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pion/opus"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/zaf/g711"

	"open-call/internal/ports/dto"
)

const agentSampleRate = 48000

// PeerSession 服务端 Offer 模式下的 WebRTC 会话。
type PeerSession struct {
	pc         *webrtc.PeerConnection
	localTrack *webrtc.TrackLocalStaticSample
	opusDec    opus.Decoder
	downlink   string
	onRemote   func(pcm []int16, sampleRate int)
	mu         sync.Mutex
	closed     bool
}

func newPeerSession(turn dto.TURNConfig, widebandWebRTC bool, onICE func(*webrtc.ICECandidate)) (*PeerSession, error) {
	servers := []webrtc.ICEServer{}
	if len(turn.STUNURLs) > 0 {
		servers = append(servers, webrtc.ICEServer{URLs: turn.STUNURLs})
	}
	if len(turn.URLs) > 0 {
		servers = append(servers, webrtc.ICEServer{
			URLs:       turn.URLs,
			Username:   turn.Username,
			Credential: turn.Credential,
		})
	}
	pc, err := webrtc.NewAPI().NewPeerConnection(webrtc.Configuration{ICEServers: servers})
	if err != nil {
		return nil, err
	}
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000},
		"audio", "aibot",
	)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	if _, err = pc.AddTrack(track); err != nil {
		_ = pc.Close()
		return nil, err
	}
	dec, _ := opus.NewDecoderWithOutput(agentSampleRate, 1)
	downlink := "PCMU/8000"
	if widebandWebRTC {
		downlink = "PCMU/8000(wideband_pending_opus_encoder)"
	}
	s := &PeerSession{pc: pc, localTrack: track, opusDec: dec, downlink: downlink}
	pc.OnICECandidate(onICE)
	pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if tr.Kind() != webrtc.RTPCodecTypeAudio {
			return
		}
		go s.readRemoteTrack(tr)
	})
	return s, nil
}

func (s *PeerSession) readRemoteTrack(tr *webrtc.TrackRemote) {
	pcmBuf := make([]byte, 5760*2)
	for {
		pkt, _, err := tr.ReadRTP()
		if err != nil {
			return
		}
		var mono []int16
		rate := agentSampleRate
		switch tr.Codec().MimeType {
		case webrtc.MimeTypeOpus:
			_, _, err := s.opusDec.Decode(pkt.Payload, pcmBuf)
			if err != nil {
				continue
			}
			mono = bytesLEToMono(pcmBuf[:960*2])
		case webrtc.MimeTypePCMU:
			rate = 8000
			mono = make([]int16, len(pkt.Payload))
			for i, b := range pkt.Payload {
				mono[i] = g711.DecodeUlawFrame(b)
			}
		default:
			continue
		}
		s.mu.Lock()
		fn := s.onRemote
		s.mu.Unlock()
		if fn != nil {
			fn(mono, rate)
		}
	}
}

func bytesLEToMono(b []byte) []int16 {
	if len(b) < 2 {
		return nil
	}
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(int(b[2*i]) | int(b[2*i+1])<<8)
	}
	return out
}

// DownlinkCodec 当前 Bot 下行 WebRTC 编解码描述（用于诊断日志）。
func (s *PeerSession) DownlinkCodec() string {
	if s == nil {
		return ""
	}
	return s.downlink
}

func (s *PeerSession) SetRemotePCMHandler(fn func([]int16, int)) {
	s.mu.Lock()
	s.onRemote = fn
	s.mu.Unlock()
}

func (s *PeerSession) CompleteOffer(offerSDP string) error {
	if err := s.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}); err != nil {
		return err
	}
	answer, err := s.pc.CreateAnswer(nil)
	if err != nil {
		return err
	}
	if err := s.pc.SetLocalDescription(answer); err != nil {
		return err
	}
	<-webrtc.GatheringCompletePromise(s.pc)
	return nil
}

func (s *PeerSession) LocalDescription() *webrtc.SessionDescription {
	return s.pc.LocalDescription()
}

// WaitConnected 等待本地 PeerConnection 进入 connected（DTLS 就绪，便于 Switch 桥接）。
func (s *PeerSession) WaitConnected(ctx context.Context) error {
	if s.pc == nil {
		return fmt.Errorf("peer 未初始化")
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		switch s.pc.ConnectionState() {
		case webrtc.PeerConnectionStateConnected:
			return nil
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			return fmt.Errorf("webrtc 连接失败: %s", s.pc.ConnectionState())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// WritePCMU8k 发送 8kHz 单声道 PCM（按 20ms/160 样本分帧）。
func (s *PeerSession) WritePCMU8k(pcm []int16) error {
	const frame = 160
	for off := 0; off < len(pcm); {
		end := off + frame
		if end > len(pcm) {
			end = len(pcm)
		}
		if err := writePCMUFrame(s.localTrack, pcm[off:end]); err != nil {
			return err
		}
		off += frame
	}
	return nil
}

func writePCMUFrame(track *webrtc.TrackLocalStaticSample, pcm []int16) error {
	const frame = 160
	if track == nil {
		return nil
	}
	buf := make([]int16, frame)
	copy(buf, pcm)
	payload := make([]byte, frame)
	for i := 0; i < frame; i++ {
		payload[i] = g711.EncodeUlawFrame(buf[i])
	}
	return track.WriteSample(media.Sample{Data: payload, Duration: 20 * time.Millisecond})
}

func (s *PeerSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	if s.pc != nil {
		_ = s.pc.Close()
	}
}
