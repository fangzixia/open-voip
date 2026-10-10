//go:build linux && cgo

package media

import (
	"fmt"
	"math"
	"math/rand"
	"net"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/rtp"
)

// Correlate every received PCM window with a known pseudorandom source. This
// measures ingest-to-UDP delivery throughout the call, rather than first sound.
// Endpoint capture/playback and carrier delays are deliberately outside it.
func TestMediaContinuousLatencyWithJitter(t *testing.T) {
	for _, jitterMS := range []int{0, 20, 40} {
		t.Run(fmt.Sprintf("jitter_%dms", jitterMS), func(t *testing.T) {
			const count = 300
			rng := rand.New(rand.NewSource(179))
			reference := make([]int16, count*160)
			for at := 0; at < len(reference); at += 8 {
				value := int16(10000)
				if rng.Intn(2) == 0 {
					value = -value
				}
				for i := at; i < min(at+8, len(reference)); i++ {
					reference[i] = value
				}
			}
			arrivals := make([]replayArrival, count)
			for i := range arrivals {
				arrivals[i] = replayArrival{at: time.Duration(i*20+rng.Intn(jitterMS+1)) * time.Millisecond, packet: &rtp.Packet{Header: rtp.Header{Version: 2, SSRC: 9, PayloadType: 0, SequenceNumber: uint16(i), Timestamp: uint32(i * 160)}, Payload: pcmToPCMU(reference[i*160 : (i+1)*160])}}
			}
			sort.SliceStable(arrivals, func(i, j int) bool { return arrivals[i].at < arrivals[j].at })
			t.Logf("initial arrivals: seq=%d at=%s seq=%d at=%s seq=%d at=%s", arrivals[0].packet.SequenceNumber, arrivals[0].at, arrivals[1].packet.SequenceNumber, arrivals[1].at, arrivals[2].packet.SequenceNumber, arrivals[2].at)
			var arrivalTimes [count]atomic.Int64
			receiver, unused := udpSocket(t), udpSocket(t)
			m := newScheduledRoomMixer()
			r := &room{mixAudio: true, sipAudio: true, mixer: m, peers: map[string]*peer{}, sipRTP: map[*sipRTP]struct{}{}}
			for j, destination := range []*net.UDPConn{unused, receiver} {
				rt := &sipRTP{legID: fmt.Sprint(j), conn: udpSocket(t), remote: destination.LocalAddr().(*net.UDPAddr), remotePT: uint8(j * 8), remoteCodec: sipAudioCodec(j * 8)}
				r.sipRTP[rt] = struct{}{}
			}
			s := &Service{}
			go s.runRoomMixLoop("latency", r, m)
			defer m.stop()
			type observation struct {
				at  time.Time
				pcm []int16
			}
			var observations []observation
			done := make(chan struct{})
			go func() {
				defer close(done)
				buf := make([]byte, 1500)
				for {
					n, _, err := receiver.ReadFromUDP(buf)
					if err != nil {
						return
					}
					var p rtp.Packet
					if p.Unmarshal(buf[:n]) == nil && p.PayloadType == 8 && len(p.Payload) == 160 {
						observations = append(observations, observation{at: time.Now(), pcm: pcmuPayloadToPCM(rtpPayloadToPCMU(8, p.Payload))})
					}
				}
			}()
			start := time.Now()
			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()
			next := 0
			var sourceLateMax time.Duration
			for time.Since(start) < 6300*time.Millisecond {
				<-tick.C
				elapsed := time.Since(start)
				for next < len(arrivals) && arrivals[next].at <= elapsed {
					arrival := arrivals[next]
					sourceLateMax = max(sourceLateMax, elapsed-arrival.at)
					arrivalTimes[int(arrival.packet.SequenceNumber)].Store(time.Now().UnixNano())
					m.ingestPacket("0", arrival.packet)
					next++
				}
			}
			q := m.quality(nil)
			m.stop()
			_ = receiver.Close()
			<-done
			var delays []float64
			previousPosition := -1
			for _, observation := range observations {
				center := int(observation.at.Sub(start).Seconds()*8000) - 480
				low, high := max(0, center-800), min(len(reference)-160, center+320)
				var energy float64
				for _, v := range observation.pcm {
					energy += math.Abs(float64(v))
				}
				if energy < 160*1000 {
					continue
				}
				best, position := -1.0, -1
				for at := low; at <= high; at++ {
					arrival := arrivalTimes[at/160].Load()
					if arrival == 0 || arrival > observation.at.UnixNano() {
						continue
					}
					var correlation float64
					for i, value := range observation.pcm {
						if reference[at+i] > 0 {
							correlation += float64(value)
						} else {
							correlation -= float64(value)
						}
					}
					correlation /= energy
					if correlation > best {
						best, position = correlation, at
					}
				}
				if best < .97 || position < 1600 || position >= len(reference)-1600 {
					continue
				}
				if previousPosition >= 0 && (position-previousPosition < 140 || position-previousPosition > 180) {
					t.Fatalf("steady PCM repeated or skipped: sample positions %d -> %d", previousPosition, position)
				}
				previousPosition = position
				delays = append(delays, float64(observation.at.UnixNano()-arrivalTimes[position/160].Load())/1e6)
				if len(delays) < 4 {
					t.Logf("matched at=%s sample=%d packet_arrival=%s score=%.4f", observation.at.Sub(start), position, time.Unix(0, arrivalTimes[position/160].Load()).Sub(start), best)
				}
			}
			sort.Float64s(delays)
			if len(delays) < count-24 {
				t.Fatalf("insufficient correlated steady windows: %d/%d", len(delays), count)
			}
			p95, p99 := delays[int(math.Ceil(float64(len(delays))*.95))-1], delays[int(math.Ceil(float64(len(delays))*.99))-1]
			t.Logf("ingest_to_udp_delivery windows=%d jitter_ms=%d p95_ms=%.2f p99_ms=%.2f source_max_lateness_ms=%.2f", len(delays), jitterMS, p95, p99, float64(sourceLateMax)/float64(time.Millisecond))
			if p95 > 100 || p99 > 140 || sourceLateMax > 5*time.Millisecond || q.OutputDropped != 0 || q.Streams["0"].OverflowSamples != 0 {
				t.Fatal("continuous processing latency or controlled source gate failed", q)
			}
		})
	}
}
