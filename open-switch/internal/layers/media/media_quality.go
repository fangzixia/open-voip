package media

import (
	"context"
	"log/slog"
	"open-switch/internal/observability"
	"sync/atomic"
	"time"
)

// Millisecond bins bound storage for calls of arbitrary duration.
type durationHistogram struct{ bins [1001]atomic.Uint64 }

func (h *durationHistogram) observe(d time.Duration) {
	h.bins[min(1000, max(0, int((d+time.Millisecond-1)/time.Millisecond)))].Add(1)
}
func (h *durationHistogram) percentile(p float64) int {
	var total uint64
	for i := range h.bins {
		total += h.bins[i].Load()
	}
	if total == 0 {
		return 0
	}
	var n uint64
	for i := range h.bins {
		n += h.bins[i].Load()
		if float64(n) >= float64(total)*p {
			return i
		}
	}
	return 1000
}

type streamQuality struct {
	SSRC            uint32  `json:"ssrc"`
	Reordered       uint64  `json:"reordered"`
	Duplicate       uint64  `json:"duplicate"`
	Expired         uint64  `json:"expired"`
	OrderedMissing  uint64  `json:"ordered_missing"`
	PLCSamples      uint64  `json:"plc_samples"`
	LateSamples     uint64  `json:"late_samples"`
	OverflowSamples uint64  `json:"overflow_samples"`
	BufferedSamples int     `json:"buffered_samples"`
	ClockPPM        float64 `json:"clock_ppm"`
	NativeError     string  `json:"native_error,omitempty"`
	Resyncs         uint64  `json:"resyncs"`
}
type mediaQualityEvent struct {
	callID string
	stats  roomQuality
	emit   func()
}
type roomQuality struct {
	Streams              map[string]streamQuality `json:"streams"`
	Outputs              map[string]outputQuality `json:"outputs"`
	OutputQueued         int                      `json:"output_queued"`
	RecordingQueued      int                      `json:"recording_queued"`
	OutputDropped        uint64                   `json:"output_dropped"`
	SchedulerP95MS       int                      `json:"scheduler_p95_ms"`
	SchedulerP99MS       int                      `json:"scheduler_p99_ms"`
	RecordingStatus      string                   `json:"recording_status"`
	RecordingFailure     string                   `json:"recording_failure,omitempty"`
	RecordingWriteErrors uint64                   `json:"recording_write_errors"`
}
type outputQuality struct {
	Queued  int    `json:"queued"`
	Dropped uint64 `json:"dropped"`
}

func (m *scheduledRoomMixer) quality(rec *recorder) roomQuality {
	q := roomQuality{Streams: map[string]streamQuality{}, Outputs: map[string]outputQuality{}, SchedulerP95MS: m.scheduling.percentile(.95), SchedulerP99MS: m.scheduling.percentile(.99)}
	m.mu.Lock()
	legs := map[string]*legPlayoutBuffer{}
	for id, b := range m.legs {
		legs[id] = b
	}
	for id, o := range m.outputs {
		o.mu.Lock()
		q.Outputs[id] = outputQuality{Queued: len(o.queue), Dropped: o.dropped}
		q.OutputDropped += o.dropped
		q.OutputQueued += len(o.queue)
		o.mu.Unlock()
	}
	m.mu.Unlock()
	for id, b := range legs {
		b.mu.Lock()
		st := streamQuality{SSRC: 0, PLCSamples: b.plcSamples, LateSamples: b.lateSamples, OverflowSamples: b.overflowSamples, BufferedSamples: len(b.samples) + len(b.outputPending), ClockPPM: b.clockPPM, Resyncs: b.resyncs}
		if b.anchored {
			st.SSRC = b.ssrc
		}
		if b.nativeError != nil {
			st.NativeError = b.nativeError.Error()
		}
		b.mu.Unlock()
		ordered := b.jitter.Stats()
		st.Reordered = ordered.PacketsReordered
		st.Duplicate = ordered.PacketsDuplicate
		st.Expired = ordered.PacketsExpired
		st.OrderedMissing = ordered.PacketsLost
		q.Streams[id] = st
	}
	if rec != nil && rec.conversation != nil {
		c := rec.conversation
		c.mu.Lock()
		q.RecordingQueued = len(c.queue)
		q.RecordingStatus = c.status
		q.RecordingFailure = c.reason
		q.RecordingWriteErrors = c.writeErrors
		c.mu.Unlock()
	}
	return q
}
func (s *Service) sampleRoomQuality(callID string, r *room, m *scheduledRoomMixer) {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-m.done():
			return
		case <-tick.C:
			r.mu.RLock()
			rec := r.rec
			r.mu.RUnlock()
			e := mediaQualityEvent{callID: callID, stats: m.quality(rec)}
			s.enqueueMediaObservation(e)
		}
	}
}

// Logging from transport callbacks must never delay receiving the first audio
// packet. One bounded consumer also isolates a congested log sink from ticks.
func (s *Service) enqueueMediaObservation(e mediaQualityEvent) {
	s.qualityOnce.Do(func() {
		s.qualityQueue = make(chan mediaQualityEvent, 32)
		go func() {
			for event := range s.qualityQueue {
				if event.emit != nil {
					event.emit()
				} else {
					slog.Info("quality.media", "call_id", event.callID, "component", "media", "event", "quality.media", "stats", event.stats)
				}
			}
		}()
	})
	select {
	case s.qualityQueue <- e:
	default:
	}
}

func (s *Service) observeTransport(ctx context.Context, event, phase string, attrs ...any) {
	s.enqueueMediaObservation(mediaQualityEvent{emit: func() {
		observability.Event(ctx, "webrtc", event, phase, "ok", "", time.Time{}, attrs...)
	}})
}
