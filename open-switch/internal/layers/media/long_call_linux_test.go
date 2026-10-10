//go:build linux && cgo

package media

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-audio/wav"
	"github.com/pion/rtp"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

// This uses wall time and the actual SDK room clock. Short invocations are
// smoke tests only; the release long-call gate requires a full two hours.
func TestMediaWallClockDrift(t *testing.T) {
	value := os.Getenv("OPEN_VOIP_MEDIA_LONG_DURATION")
	if value == "" {
		t.Skip("set OPEN_VOIP_MEDIA_LONG_DURATION=2h for wall-clock drift gate")
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 2*time.Second {
		t.Fatal("invalid long-call duration")
	}
	ctx := context.Background()
	receiver := udpSocket(t)
	var received, invalid atomic.Uint64
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
			if p.Unmarshal(buf[:n]) != nil || len(p.Payload) != 160 || (p.PayloadType != 0 && p.PayloadType != 8) {
				invalid.Add(1)
			} else {
				received.Add(1)
			}
		}
	}()
	m := newScheduledRoomMixer()
	r := &room{mixAudio: true, sipAudio: true, mixer: m, peers: map[string]*peer{}, sipRTP: map[*sipRTP]struct{}{}}
	for j := range 2 {
		c := udpSocket(t)
		r.sipRTP[&sipRTP{legID: fmt.Sprint(j), conn: c, remote: receiver.LocalAddr().(*net.UDPAddr), remotePT: uint8(j * 8), remoteCodec: sipAudioCodec(j * 8)}] = struct{}{}
	}
	s := &Service{audioRecDir: t.TempDir(), rooms: map[string]*room{"drift": r}, recByID: map[string]*recorder{}}
	go s.runRoomMixLoop("drift", r, m)
	defer m.stop()
	id, err := s.StartRecording(ctx, "drift", dto.RecordingPolicy{Mode: "audio"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.StopRecording(ctx, id)
	start := time.Now()
	tick := time.NewTicker(rtpFrameDur)
	defer tick.Stop()
	var submitted [2]uint64
	ppm := [2]float64{-100, 100}
	var maxBuffered [2]int
	var correctionSum [2]float64
	var correctionCount int
	var baselinePLC uint64
	var baselineSet bool
	var sourceLateMax time.Duration
	nextObservation := start.Add(time.Second)
	for time.Since(start) < duration {
		scheduled := <-tick.C
		sourceLateMax = max(sourceLateMax, time.Since(scheduled))
		elapsed := time.Since(start)
		for j := range 2 {
			wanted := uint64(math.Floor(elapsed.Seconds()*50*(1+ppm[j]/1e6))) + 3
			for submitted[j] < wanted {
				n := submitted[j]
				m.ingestPacket(fmt.Sprint(j), &rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: uint8(j * 8), SSRC: uint32(j + 1), SequenceNumber: uint16(n), Timestamp: uint32(n * 160)}, Payload: pcmToG711(constantPCM(int16(3000+j*3000)), uint8(j*8))})
				submitted[j]++
			}
		}
		if time.Now().Before(nextObservation) {
			continue
		}
		nextObservation = time.Now().Add(time.Second)
		q := m.quality(s.recByID[id])
		if q.RecordingStatus == "failed" || q.OutputDropped != 0 {
			t.Fatal("recording or output failed during long call", q)
		}
		var concealed uint64
		for j := range 2 {
			st := q.Streams[fmt.Sprint(j)]
			maxBuffered[j] = max(maxBuffered[j], st.BufferedSamples)
			concealed += st.PLCSamples
			if st.OverflowSamples != 0 || st.Resyncs != 0 || st.NativeError != "" || st.BufferedSamples > defaultPlayoutCap {
				t.Fatal("clock drift exhausted bounded pipeline", st)
			}
			if elapsed >= duration/2 {
				correctionSum[j] += st.ClockPPM
			}
		}
		if elapsed >= duration/2 {
			correctionCount++
		}
		if elapsed >= 5*time.Second {
			if !baselineSet {
				baselinePLC, baselineSet = concealed, true
			} else if concealed != baselinePLC {
				t.Fatal("steady drift required PLC despite loss-free input", q.Streams)
			}
		}
	}
	m.stop()
	_ = receiver.Close()
	<-done
	if err = s.StopRecording(ctx, id); err != nil {
		t.Fatal(err)
	}
	meta, err := s.RecordingInfo(ctx, id)
	if err != nil || meta.Status != "completed" || len(meta.LegPaths) != 2 {
		t.Fatal("incomplete long-call recording", meta, err)
	}
	assertRecordingWAVAlignment(t, meta)
	for j := range 2 {
		mean := correctionSum[j] / float64(max(1, correctionCount))
		t.Logf("duration=%s input_ppm=%.0f latter_half_mean_correction_ppm=%.2f max_buffered_samples=%d submitted=%d", duration, ppm[j], mean, maxBuffered[j], submitted[j])
		if duration >= 2*time.Hour && math.Abs(mean-ppm[j]) > 30 {
			t.Fatal("real-time correction did not track input clock")
		}
	}
	t.Logf("duration_samples=%d udp_packets=%d invalid_packets=%d producer_max_lateness_ms=%.2f", meta.DurationSamples, received.Load(), invalid.Load(), float64(sourceLateMax)/float64(time.Millisecond))
	if invalid.Load() != 0 || received.Load() < uint64(duration.Seconds()*80) || sourceLateMax > 40*time.Millisecond {
		t.Fatal("wall-clock media or controlled source gate failed")
	}
}

// Inspect headers without loading hours of PCM into memory.
func assertRecordingWAVAlignment(t *testing.T, meta ports.RecordingMeta) {
	t.Helper()
	paths := []string{meta.FilePath}
	for _, path := range meta.LegPaths {
		paths = append(paths, path)
	}
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		d := wav.NewDecoder(f)
		err = d.FwdToPCM()
		_ = f.Close()
		if err != nil || int64(d.PCMSize)/2 != meta.DurationSamples || d.SampleRate != 8000 || d.NumChans != 1 || d.BitDepth != 16 {
			t.Fatal("long-call WAVs lost shared sample alignment", path, d.PCMSize, meta.DurationSamples, err)
		}
	}
}
