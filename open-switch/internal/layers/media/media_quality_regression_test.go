package media

import (
	"fmt"
	"github.com/livekit/media-sdk/jitter"
	"github.com/pion/rtp"
	"math"
	"testing"
	"time"
)

func TestPlayoutTwoHourClockDriftSimulation(t *testing.T) {
	for _, ppm := range []float64{-100, 100} {
		t.Run(fmt.Sprint(ppm), func(t *testing.T) {
			b := newLegPlayoutBuffer(defaultPlayoutCap)
			defer b.close()
			now := time.Now()
			b.now = func() time.Time { return now }
			pushed, maxQueued := 0, 0
			var correctionSum float64
			for tick := 0; tick < 360000; tick++ {
				now = now.Add(20 * time.Millisecond)
				count := int(math.Floor(float64(tick+3) * (1 + ppm/1e6)))
				for pushed < count {
					p := &rtp.Packet{Header: rtp.Header{SSRC: 1, SequenceNumber: uint16(pushed), Timestamp: uint32(pushed * 160), PayloadType: 127}, Payload: encodePCM16(constantPCM(2000))}
					b.accept([]jitter.ExtPacket{{Packet: p, ReceivedAt: now}})
					pushed++
				}
				out := b.pullAdaptiveFrame()
				if len(out) != 0 && len(out) != 160 {
					t.Fatalf("output length %d", len(out))
				}
				b.mu.Lock()
				maxQueued = max(maxQueued, len(b.samples)+len(b.outputPending))
				b.mu.Unlock()
				if tick >= 180000 {
					correctionSum += b.clockPPM
				}
			}
			t.Logf("input_ppm=%.0f instantaneous_correction_ppm=%.2f last_hour_mean_correction_ppm=%.2f maximum_queue_samples=%d plc_samples=%d", ppm, b.clockPPM, correctionSum/180000, maxQueued, b.plcSamples)
			if maxQueued > defaultPlayoutCap || b.overflowSamples != 0 {
				t.Fatal("clock drift accumulated latency or overflow")
			}
			if math.Abs(correctionSum/180000-ppm) > 30 {
				t.Fatal("clock correction did not track the input")
			}
			if b.plcSamples != 0 {
				t.Fatal("loss-free input required concealment")
			}
		})
	}
}
func TestPlayoutDuplicateDoesNotBecomeHugeLoss(t *testing.T) {
	b := newLegPlayoutBuffer(defaultPlayoutCap)
	defer b.close()
	b.ingest(7, 1, 0, constantPCM(123))
	b.ingest(7, 1, 0, constantPCM(456))
	b.ingest(8, 1, 160, constantPCM(789))
	waitPlayout(t, b)
	if b.pullFrame()[0] != 123 || b.pullFrame()[0] != 789 {
		t.Fatal("duplicate displaced ordered audio")
	}
	stats := b.jitter.Stats()
	if stats.PacketsDuplicate != 1 || stats.PacketsLost != 0 {
		t.Fatalf("duplicate incorrectly counted as loss: %+v", stats)
	}
}

func TestPlayoutStartupReserveUsesOrderedArrivalClock(t *testing.T) {
	b := newLegPlayoutBuffer(defaultPlayoutCap)
	defer b.close()
	base := time.Now()
	b.readyAt = base.Add(34*time.Millisecond + inputPCMReserve)
	for i, arrival := range []time.Duration{34 * time.Millisecond, 40 * time.Millisecond, 54 * time.Millisecond} {
		b.accept([]jitter.ExtPacket{{Packet: &rtp.Packet{Header: rtp.Header{SSRC: 1, SequenceNumber: uint16(i), Timestamp: uint32(i * 160), PayloadType: 127}, Payload: encodePCM16(constantPCM(1000))}, ReceivedAt: base.Add(arrival)}})
	}
	if want := base.Add(14*time.Millisecond + inputPCMReserve); !b.readyAt.Equal(want) {
		t.Fatalf("first network delay stacked with reserve: got %s want %s", b.readyAt.Sub(base), want.Sub(base))
	}
	// Once samples are consumed, a late arrival cannot move the playback epoch.
	b.cursor = 160
	ready := b.readyAt
	b.accept([]jitter.ExtPacket{{Packet: &rtp.Packet{Header: rtp.Header{SSRC: 1, SequenceNumber: 3, Timestamp: 480, PayloadType: 127}, Payload: encodePCM16(constantPCM(1000))}, ReceivedAt: base.Add(60 * time.Millisecond)}})
	if !b.readyAt.Equal(ready) {
		t.Fatal("active epoch moved during playback")
	}
}
func TestOutputHeadroomAndRouteTransition(t *testing.T) {
	g := &routeGain{}
	one := g.mix(map[string][]int16{"a": constantPCM(32767)})
	if one[0] >= one[159] || one[159] > 27853 {
		t.Fatal("20 ms transition/headroom missing")
	}
	both := g.mix(map[string][]int16{"a": constantPCM(32767), "b": constantPCM(32767)})
	for _, v := range both {
		if v < 0 || v > 27853 {
			t.Fatalf("mix overflow or gain exceeded .85: %d", v)
		}
	}
}
func TestClearDropsNativeResamplerHistory(t *testing.T) {
	o := &audioOutput{generation: 1}
	if e := o.writeRate(1, makeTone(480, 48000, 1000), 48000); e != nil {
		t.Fatal(e)
	}
	if e := o.clear(2); e != nil {
		t.Fatal(e)
	}
	if e := o.writeRate(2, make([]int16, 480), 48000); e != nil {
		t.Fatal(e)
	}
	if e := o.finish(2); e != nil {
		t.Fatal(e)
	}
	frame, _, _ := o.pull(time.Now())
	for _, v := range frame {
		if v != 0 {
			t.Fatal("cancelled generation leaked into new audio")
		}
	}
}
func makeTone(n, rate int, hz float64) []int16 {
	p := make([]int16, n)
	for i := range p {
		p[i] = int16(10000 * math.Sin(2*math.Pi*hz*float64(i)/float64(rate)))
	}
	return p
}
func TestSIPUnsupportedAnswerDoesNotMutateSession(t *testing.T) {
	r := &sipRTP{remotePT: 8, remoteCodec: sipCodecPCMA}
	if e := applyRemoteSDP(r, parseSDP(testSDPHeader+"m=audio 9000 RTP/AVP 111\r\na=rtpmap:111 opus/48000/2\r\n")); e == nil {
		t.Fatal("unsupported answer accepted")
	}
	if r.currentPT() != 8 || r.currentCodec() != sipCodecPCMA {
		t.Fatal("failed negotiation changed the active codec")
	}
}

func TestSIPStaticPayloadMappingMustMatchCodecAndClock(t *testing.T) {
	for _, mapping := range []string{"opus/48000/2", "PCMU/16000", "PCMA/8000", "PCMU/8000/2"} {
		offer := parseSDP(testSDPHeader + "m=audio 9000 RTP/AVP 0\r\na=rtpmap:0 " + mapping + "\r\n")
		if offer.hasAudioCodec() || buildAnswerSDP("127.0.0.1", 9001, offer) != "" {
			t.Fatalf("accepted contradictory mapping: %s", mapping)
		}
	}
	valid := parseSDP(testSDPHeader + "m=audio 9000 RTP/AVP 0 8\r\na=rtpmap:0 opus/48000/2\r\na=rtpmap:8 PCMA/8000\r\n")
	if !valid.hasAudioCodec() || valid.preferG711() != 8 {
		t.Fatal("valid common PCMA was not selected")
	}
}
