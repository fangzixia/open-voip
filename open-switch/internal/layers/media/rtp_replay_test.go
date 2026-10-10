package media

import (
	"fmt"
	"github.com/pion/rtp"
	"math/rand"
	"sort"
	"testing"
	"time"
)

type replayArrival struct {
	at     time.Duration
	packet *rtp.Packet
}

func TestPlayoutPersistentDelayStepRecoversRealAudio(t *testing.T) {
	b := newLegPlayoutBuffer(defaultPlayoutCap)
	defer b.close()
	start := time.Now()
	var next int
	recovered := 0
	clock := time.NewTicker(20 * time.Millisecond)
	defer clock.Stop()
	for time.Since(start) < 1500*time.Millisecond {
		elapsed := time.Since(start)
		for next < 65 {
			at := time.Duration(next) * 20 * time.Millisecond
			if next >= 15 {
				at += 80 * time.Millisecond
			}
			if elapsed < at {
				break
			}
			pcm := makeTone(160, 8000, 400)
			if next >= 15 {
				pcm = constantPCM(9000)
			}
			b.push(&rtp.Packet{Header: rtp.Header{SSRC: 1, SequenceNumber: uint16(next), Timestamp: uint32(next * 160), PayloadType: 0}, Payload: pcmToPCMU(pcm)})
			next++
		}
		select {
		case <-clock.C:
			out := b.pullAdaptiveFrame()
			if elapsed > 700*time.Millisecond && elapsed < 1250*time.Millisecond {
				var sum int64
				for _, v := range out {
					sum += int64(v)
				}
				if len(out) == 160 && sum/160 > 7000 {
					recovered++
				}
			}
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if recovered < 15 || b.resyncs == 0 {
		t.Fatalf("persistent phase change did not recover actual input: recovered=%d resyncs=%d", recovered, b.resyncs)
	}
	if b.overflowSamples != 0 || len(b.samples)+len(b.outputPending) > defaultPlayoutCap {
		t.Fatal("recovery exceeded PCM budget")
	}
}

// Fixed seeds make perturbations reproducible; delays use wall time so the
// frozen jitter implementation's actual expiration timer is exercised.
func TestRTPDisturbanceReplay(t *testing.T) {
	for _, jitterMS := range []int{0, 20, 40, 80} {
		for _, lossPercent := range []int{0, 1, 3} {
			t.Run(fmt.Sprintf("jitter_%d_loss_%d", jitterMS, lossPercent), func(t *testing.T) {
				b := newLegPlayoutBuffer(defaultPlayoutCap)
				defer b.close()
				random := rand.New(rand.NewSource(59))
				var arrivals []replayArrival
				const packets = 60
				for i := 0; i < packets; i++ {
					if lossPercent > 0 && (random.Intn(100) < lossPercent || (i >= 25 && i < 28)) {
						continue
					}
					extra := 0
					if jitterMS > 0 {
						extra = random.Intn(jitterMS + 1)
					}
					pcm := makeTone(160, 8000, 400)
					if i >= 40 {
						pcm = constantPCM(9000)
					}
					p := &rtp.Packet{Header: rtp.Header{SSRC: 42, SequenceNumber: uint16(65520 + i), Timestamp: uint32(0xffffe000 + uint64(i*160)), PayloadType: 0}, Payload: pcmToPCMU(pcm)}
					at := time.Duration(i*20+extra) * time.Millisecond
					arrivals = append(arrivals, replayArrival{at, p})
					if i%13 == 0 {
						arrivals = append(arrivals, replayArrival{at + time.Millisecond, p.Clone()})
					}
				}
				sort.SliceStable(arrivals, func(i, j int) bool { return arrivals[i].at < arrivals[j].at })
				start := time.Now()
				next, activeTail := 0, 0
				tick := time.NewTicker(20 * time.Millisecond)
				defer tick.Stop()
				for time.Since(start) < 1400*time.Millisecond {
					elapsed := time.Since(start)
					for next < len(arrivals) && arrivals[next].at <= elapsed {
						b.push(arrivals[next].packet)
						next++
					}
					select {
					case <-tick.C:
						out := b.pullAdaptiveFrame()
						if len(out) != 0 && len(out) != 160 {
							t.Fatal("packetization reached output")
						}
						if elapsed > time.Second && elapsed < 1250*time.Millisecond {
							var sum int64
							for _, v := range out {
								sum += int64(v)
							}
							if len(out) == 160 && sum/160 > 7000 {
								activeTail++
							}
						}
					default:
						time.Sleep(time.Millisecond)
					}
				}
				q := b.jitter.Stats()
				t.Logf("reordered=%d duplicate=%d expired=%d ordered_missing=%d plc_samples=%d late_samples=%d", q.PacketsReordered, q.PacketsDuplicate, q.PacketsExpired, q.PacketsLost, b.plcSamples, b.lateSamples)
				if activeTail < 3 {
					t.Fatal("audio did not recover after perturbation")
				}
				if b.overflowSamples != 0 || len(b.samples) > defaultPlayoutCap {
					t.Fatal("unbounded timeline/queue")
				}
			})
		}
	}
}
