//go:build linux && cgo

package media

import (
	"context"
	"fmt"
	"github.com/pion/rtp"
	"math"
	"net"
	"open-switch/internal/ports/dto"
	"os"
	"runtime"
	"sort"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// This is a component load gate: real SDK clocks, both G.711 codecs, UDP
// output, main/stem WAVs. SIP signaling, browser and device gates are separate.
func TestMediaCapacity(t *testing.T) {
	value := os.Getenv("OPEN_VOIP_MEDIA_LOAD_DURATION")
	if value == "" {
		t.Skip("set OPEN_VOIP_MEDIA_LOAD_DURATION=60m for capacity gate")
	}
	duration, e := time.ParseDuration(value)
	if e != nil || duration < time.Second {
		t.Fatal("invalid duration")
	}
	calls := 100
	runtime.GOMAXPROCS(4)
	root := t.TempDir()
	ctx := context.Background()
	s := &Service{audioRecDir: root, rooms: map[string]*room{}, recByID: map[string]*recorder{}}
	receiver, e := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if e != nil {
		t.Fatal(e)
	}
	defer receiver.Close()
	_ = receiver.SetReadBuffer(8 << 20)
	var received, badPackets atomic.Uint64
	var inputStarted atomic.Int64
	collectorDone := make(chan struct{})
	firstVoice := map[uint32]float64{}
	go func() {
		defer close(collectorDone)
		buf := make([]byte, 1500)
		for {
			n, _, e := receiver.ReadFromUDP(buf)
			if e != nil {
				return
			}
			var p rtp.Packet
			if e = p.Unmarshal(buf[:n]); e != nil || len(p.Payload) != 160 || (p.PayloadType != 0 && p.PayloadType != 8) {
				badPackets.Add(1)
			} else {
				received.Add(1)
				if started := inputStarted.Load(); started > 0 {
					if _, seen := firstVoice[p.SSRC]; !seen {
						for _, v := range pcmuPayloadToPCM(rtpPayloadToPCMU(p.PayloadType, p.Payload)) {
							if v > 500 || v < -500 {
								firstVoice[p.SSRC] = float64(time.Now().UnixNano()-started) / 1e6
								break
							}
						}
					}
				}
			}
		}
	}()
	var mixes []*scheduledRoomMixer
	var recordings []string
	for i := 0; i < calls; i++ {
		id := fmt.Sprintf("load-%03d", i)
		m := newScheduledRoomMixer()
		r := &room{mixAudio: true, sipAudio: true, mixer: m, peers: map[string]*peer{}, sipRTP: map[*sipRTP]struct{}{}}
		for j := 0; j < 2; j++ {
			c, e := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if e != nil {
				t.Fatal(e)
			}
			rt := &sipRTP{callID: id, legID: fmt.Sprint(j), conn: c, remote: receiver.LocalAddr().(*net.UDPAddr), remotePT: uint8(j * 8), remoteCodec: sipAudioCodec(j * 8)}
			r.sipRTP[rt] = struct{}{}
			defer rt.close()
		}
		s.rooms[id] = r
		mixes = append(mixes, m)
		go s.runRoomMixLoop(id, r, m)
		rec, e := s.StartRecording(ctx, id, dto.RecordingPolicy{Mode: "audio"})
		if e != nil {
			t.Fatal(e)
		}
		recordings = append(recordings, rec)
	}
	defer func() {
		for _, m := range mixes {
			m.stop()
		}
		for _, id := range recordings {
			_ = s.StopRecording(ctx, id)
		}
	}()
	usage := func() float64 {
		var u syscall.Rusage
		_ = syscall.Getrusage(syscall.RUSAGE_SELF, &u)
		return float64(u.Utime.Sec+u.Stime.Sec) + float64(u.Utime.Usec+u.Stime.Usec)/1e6
	}
	start := time.Now()
	lastCPU := usage()
	sampled := time.Now()
	var cpu []float64
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	payloads := [][]byte{pcmToG711(makeTone(160, 8000, 400), 0), pcmToG711(makeTone(160, 8000, 600), 8)}
	var submitted uint64
	var sourceScheduling durationHistogram
	var sourceLateMax time.Duration
	var healthyStartPLC, healthyEndPLC uint64
	healthyStarted := false
	nextProgress := start.Add(time.Minute)
	for time.Since(start) < duration {
		scheduled := <-ticker.C
		late := time.Since(scheduled)
		sourceScheduling.observe(late)
		sourceLateMax = max(sourceLateMax, late)
		if inputStarted.Load() == 0 {
			inputStarted.Store(time.Now().UnixNano())
		}
		// A Go ticker may coalesce ticks. Preserve every source sample instead
		// of silently slowing its RTP clock when a producer wake-up is late.
		wanted := uint64(time.Since(start) / rtpFrameDur)
		for submitted < wanted {
			for _, m := range mixes {
				for j, payload := range payloads {
					m.ingestPacket(fmt.Sprint(j), &rtp.Packet{Header: rtp.Header{SSRC: uint32(j + 1), SequenceNumber: uint16(submitted), Timestamp: uint32(submitted * 160), PayloadType: uint8(j * 8)}, Payload: payload})
				}
			}
			submitted++
		}
		if time.Since(sampled) >= time.Second {
			nowCPU := usage()
			cpu = append(cpu, 100*(nowCPU-lastCPU)/time.Since(sampled).Seconds()/4)
			lastCPU = nowCPU
			sampled = time.Now()
			var concealed uint64
			for i, m := range mixes {
				q := m.quality(s.recByID[recordings[i]])
				for _, st := range q.Streams {
					concealed += st.PLCSamples
					if st.OverflowSamples != 0 || st.Resyncs != 0 || st.NativeError != "" || st.BufferedSamples > defaultPlayoutCap {
						t.Fatalf("call %d exhausted bounded pipeline: %+v", i, st)
					}
				}
				if q.RecordingStatus == "failed" {
					t.Fatalf("call %d recording failed: %s", i, q.RecordingFailure)
				}
				if q.OutputDropped > 0 {
					t.Fatalf("call %d sender overflow", i)
				}
			}
			if time.Since(start) >= 5*time.Second && !healthyStarted {
				healthyStartPLC = concealed
				healthyStarted = true
			}
			if time.Since(start) < duration-5*time.Second {
				healthyEndPLC = concealed
			}
			if time.Now().After(nextProgress) {
				t.Logf("progress elapsed=%s latest_cpu_percent=%.2f plc_samples=%d submitted_frames=%d", time.Since(start).Round(time.Second), cpu[len(cpu)-1], concealed, submitted)
				nextProgress = time.Now().Add(time.Minute)
			}
		}
	}
	_ = receiver.Close()
	<-collectorDone
	voiceDelays := make([]float64, 0, len(firstVoice))
	for _, delay := range firstVoice {
		voiceDelays = append(voiceDelays, delay)
	}
	sort.Float64s(voiceDelays)
	delayP95, delayP99 := 0.0, 0.0
	if len(voiceDelays) > 0 {
		delayP95 = voiceDelays[int(math.Ceil(float64(len(voiceDelays))*.95))-1]
		delayP99 = voiceDelays[int(math.Ceil(float64(len(voiceDelays))*.99))-1]
	}
	maxP99, plc := 0, uint64(0)
	for i, m := range mixes {
		m.stop()
		q := m.quality(s.recByID[recordings[i]])
		maxP99 = max(maxP99, q.SchedulerP99MS)
		for _, st := range q.Streams {
			plc += st.PLCSamples
		}
		if e = s.StopRecording(ctx, recordings[i]); e != nil {
			t.Fatal(e)
		}
		meta, e := s.RecordingInfo(ctx, recordings[i])
		if e != nil || meta.Status != "completed" || len(meta.LegPaths) != 2 {
			t.Fatalf("incomplete recording: %+v %v", meta, e)
		}
		assertRecordingWAVAlignment(t, meta)
	}
	sort.Float64s(cpu)
	p95 := 0.0
	if len(cpu) > 0 {
		p95 = cpu[int(math.Ceil(float64(len(cpu))*.95))-1]
	}
	var u syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &u)
	t.Logf("calls=%d duration=%s vcpu_affinity_external gomaxprocs=4 cpu_p95_percent=%.2f scheduler_worst_call_p99_ms=%d udp_packets=%d bad_packets=%d plc_samples=%d peak_rss_kb=%d", calls, duration, p95, maxP99, received.Load(), badPackets.Load(), plc, u.Maxrss)
	t.Logf("first_voice_loopback_delay_p95_ms=%.2f p99_ms=%.2f streams=%d healthy_interval_plc_samples=%d", delayP95, delayP99, len(firstVoice), healthyEndPLC-healthyStartPLC)
	t.Logf("producer_lateness_p99_ms=%d max_ms=%.2f submitted_frames=%d", sourceScheduling.percentile(.99), float64(sourceLateMax)/float64(time.Millisecond), submitted)
	if sourceLateMax > 40*time.Millisecond {
		t.Fatal("controlled producer exceeded 40 ms jitter budget; this run cannot establish the loss-free capacity gate")
	}
	if p95 > 70 || maxP99 > 5 || badPackets.Load() != 0 {
		t.Fatal("capacity quality gate failed")
	}
	if len(firstVoice) != 2*calls || delayP95 > 100 || delayP99 > 140 {
		t.Fatal("first voice delay gate failed")
	}
	if duration > 12*time.Second && healthyEndPLC != healthyStartPLC {
		t.Fatal("loss-free media required concealment during steady input")
	}
	if received.Load() < uint64(calls*2*int(duration.Seconds())*40) {
		t.Fatal("sustained media underload")
	}
}
