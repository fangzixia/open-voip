package media

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

func TestRouteRemovalFadesOnceWithoutReplayingFrame(t *testing.T) {
	for _, sample := range []int16{12000, -12000} {
		t.Run(fmt.Sprint(sample), func(t *testing.T) {
			g := &routeGain{}
			pcm := constantPCM(sample)
			pcm[0] = -sample // Detect replay of the old frame instead of its endpoint.
			g.mix(map[string][]int16{"departed": pcm})
			fade := g.mix(nil)
			for i, v := range fade {
				if int(v)*int(sample) < 0 {
					t.Fatal("departed source replayed old frame")
				}
				if i > 0 && absSample(v) > absSample(fade[i-1]) {
					t.Fatal("source removal did not fade monotonically")
				}
			}
			if fade[0] == 0 || fade[len(fade)-1] != 0 {
				t.Fatal("source cut abruptly or fade exceeded one frame")
			}
			for _, v := range g.mix(nil) {
				if v != 0 {
					t.Fatal("removed source survived its fade frame")
				}
			}
		})
	}
}

func TestRouteSwitchUsesCurrentSourceAndPreservesHeadroom(t *testing.T) {
	g := &routeGain{}
	g.mix(map[string][]int16{"old": constantPCM(-32768)})
	// The current old-source frame has changed sign. The switch must use it,
	// rather than repeat audio from the preceding frame.
	switched := g.mixWithTransitions(map[string][]int16{"new": constantPCM(32767)}, map[string][]int16{"old": constantPCM(32767)})
	for _, v := range switched {
		if v < 27851 || v > 27853 {
			t.Fatalf("route switch replayed stale audio or exceeded headroom: %d", v)
		}
	}
	steady := g.mix(map[string][]int16{"new": constantPCM(1000)})
	for _, v := range steady {
		if v < 849 || v > 850 {
			t.Fatalf("departed route persisted into next frame: %d", v)
		}
	}
}

func absSample(v int16) int {
	if v < 0 {
		return -int(v)
	}
	return int(v)
}

// Capture Pion's bound output track without requiring an ICE connection in a
// deterministic routing test. The existing transport test covers ICE/SRTP.
type holdTrackCapture struct{ packets chan *rtp.Packet }

func (c *holdTrackCapture) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	c.packets <- (&rtp.Packet{Header: *header, Payload: payload}).Clone()
	return len(payload), nil
}
func (c *holdTrackCapture) Write(raw []byte) (int, error) {
	p := &rtp.Packet{}
	if err := p.Unmarshal(raw); err != nil {
		return 0, err
	}
	c.packets <- p
	return len(raw), nil
}

type holdTrackContext struct{ capture *holdTrackCapture }

func (c holdTrackContext) CodecParameters() []webrtc.RTPCodecParameters {
	return []webrtc.RTPCodecParameters{{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000}, PayloadType: 0}}
}
func (holdTrackContext) HeaderExtensions() []webrtc.RTPHeaderExtensionParameter { return nil }
func (holdTrackContext) SSRC() webrtc.SSRC                                      { return 555 }
func (holdTrackContext) SSRCRetransmission() webrtc.SSRC                        { return 0 }
func (holdTrackContext) SSRCForwardErrorCorrection() webrtc.SSRC                { return 0 }
func (c holdTrackContext) WriteStream() webrtc.TrackLocalWriter                 { return c.capture }
func (holdTrackContext) ID() string                                             { return "hold-output" }
func (holdTrackContext) RTCPReader() interceptor.RTCPReader                     { return nil }

func TestHeldLegKeepsRTPClockAndResumes(t *testing.T) {
	for _, endpoint := range []string{"SIP-PCMU", "SIP-PCMA", "WebRTC-PCMU"} {
		t.Run(endpoint, func(t *testing.T) {
			mix := newScheduledRoomMixer()
			t.Cleanup(mix.stop)
			r := &room{mixAudio: true, mixer: mix, peers: map[string]*peer{}, sipRTP: map[*sipRTP]struct{}{}}
			s := &Service{rooms: map[string]*room{"call": r}}
			var receive func() *rtp.Packet
			var setHeld func(bool)
			var pt uint8
			if endpoint == "WebRTC-PCMU" {
				track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000}, "audio", "call")
				if err != nil {
					t.Fatal(err)
				}
				capture := &holdTrackCapture{packets: make(chan *rtp.Packet, 16)}
				binding := holdTrackContext{capture: capture}
				if _, err = track.Bind(binding); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = track.Unbind(binding) })
				p := &peer{audioOut: track}
				r.peers["receiver"] = p
				setHeld = func(on bool) { p.held = on }
				receive = func() *rtp.Packet {
					select {
					case packet := <-capture.packets:
						return packet
					case <-time.After(time.Second):
						t.Fatal("held WebRTC receiver stopped receiving room frames")
						return nil
					}
				}
			} else {
				if endpoint == "SIP-PCMA" {
					pt = 8
				}
				sender, receiver := udpSocket(t), udpSocket(t)
				rt := &sipRTP{conn: sender, remote: receiver.LocalAddr().(*net.UDPAddr), legID: "receiver", remotePT: pt}
				r.sipRTP[rt] = struct{}{}
				setHeld = func(on bool) {
					rt.mu.Lock()
					rt.held = on
					rt.mu.Unlock()
				}
				receive = func() *rtp.Packet {
					_ = receiver.SetReadDeadline(time.Now().Add(time.Second))
					raw := make([]byte, 1500)
					n, _, err := receiver.ReadFromUDP(raw)
					if err != nil {
						t.Fatalf("held SIP receiver stopped receiving room frames: %v", err)
					}
					packet := &rtp.Packet{}
					if err = packet.Unmarshal(raw[:n]); err != nil {
						t.Fatal(err)
					}
					return packet
				}
			}
			// Cross both sequence and timestamp wraparound during hold.
			mix.outSSRC["receiver"] = 77
			mix.outSeq["receiver"] = 65534
			mix.outTS["receiver"] = ^uint32(0) - 159
			var previous *rtp.Packet
			tick := func() []int16 {
				t.Helper()
				s.dispatchMixFrames("call", r, mix, map[string][]int16{"caller": constantPCM(6000), "receiver": constantPCM(-12000)})
				packet := receive()
				if packet.PayloadType != pt || len(packet.Payload) != mixFrameSamples {
					t.Fatal("negotiated codec or 20 ms frame changed during hold")
				}
				if previous != nil && (packet.SSRC != previous.SSRC || packet.SequenceNumber != previous.SequenceNumber+1 || packet.Timestamp-previous.Timestamp != 160) {
					t.Fatalf("RTP discontinuity: previous=%+v current=%+v", previous.Header, packet.Header)
				}
				previous = packet
				return pcmuPayloadToPCM(rtpPayloadToPCMU(pt, packet.Payload))
			}
			assertVoice := func(pcm []int16) {
				t.Helper()
				if pcm[159] < 4800 || pcm[159] > 5400 {
					t.Fatalf("caller missing or receiver heard its own voice: %d", pcm[159])
				}
			}
			assertSilence := func(pcm []int16) {
				t.Helper()
				for _, v := range pcm {
					// A-law encodes zero as +/-8 after decoding.
					if absSample(v) > 8 {
						t.Fatalf("held leg received caller audio instead of silence: %d", v)
					}
				}
			}
			assertVoice(tick())
			assertVoice(tick())
			r.mu.Lock()
			setHeld(true)
			r.mu.Unlock()
			fade := tick()
			if fade[0] < 4800 || absSample(fade[159]) > 8 {
				t.Fatal("holding did not fade the active route within one frame")
			}
			for range 3 {
				assertSilence(tick())
			}
			if endpoint != "WebRTC-PCMU" {
				r.mu.Lock()
				r.prompt = &roomPrompt{target: "receiver", pcm: constantPCM(4000), loop: true, gapSamples: 160, generation: r.promptSeq.Load(), readySince: time.Now().Add(-time.Second)}
				r.mu.Unlock()
				if pcm := tick(); pcm[159] < 3100 || pcm[159] > 3700 {
					t.Fatal("held SIP receiver did not receive targeted waiting audio")
				}
				if pcm := tick(); absSample(pcm[159]) > 8 {
					t.Fatal("waiting audio did not fade into its silent gap")
				}
				assertSilence(tick())
				if pcm := tick(); pcm[159] < 3100 || pcm[159] > 3700 {
					t.Fatal("waiting audio did not resume after its gap")
				}
			}
			r.mu.Lock()
			setHeld(false)
			r.promptSeq.Add(1)
			r.mu.Unlock()
			assertVoice(tick())
			assertVoice(tick())
		})
	}
}
