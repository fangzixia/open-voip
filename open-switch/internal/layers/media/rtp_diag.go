package media

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"open-switch/internal/observability"
)

// rtpPtimeDiag 采样记录 RTP 时间戳增量与载荷样本数是否一致（相对 8 kHz 时钟）。
type rtpPtimeDiag struct {
	enabled bool
	mu      sync.Mutex
	lastTS  map[string]uint32
	have    map[string]bool
	// 每 leg 最多每 64 次包记录一次 mismatch，避免刷屏。
	skip map[string]*atomic.Uint32
}

func newRTPPtimeDiag(enabled bool) *rtpPtimeDiag {
	if !enabled {
		return nil
	}
	return &rtpPtimeDiag{
		enabled: true,
		lastTS:  map[string]uint32{},
		have:    map[string]bool{},
		skip:    map[string]*atomic.Uint32{},
	}
}

func (d *rtpPtimeDiag) observe(legID string, ts uint32, payloadSamples int, clockRate int) {
	if d == nil || !d.enabled || legID == "" || payloadSamples <= 0 || clockRate <= 0 {
		return
	}
	d.mu.Lock()
	prev, ok := d.lastTS[legID]
	d.lastTS[legID] = ts
	d.mu.Unlock()
	if !ok {
		return
	}
	delta := int(uint32(ts - prev))
	if delta <= 0 {
		return
	}
	if delta == payloadSamples {
		return
	}
	counter := d.skip[legID]
	if counter == nil {
		counter = &atomic.Uint32{}
		d.skip[legID] = counter
	}
	if counter.Add(1)%64 != 1 {
		return
	}
	ctx := observability.WithFields(context.Background(), observability.Fields{LegID: legID})
	observability.Event(ctx, "media", "rtp.ptime_mismatch", "observe", "ok", "", time.Now(),
		"rtp_ts_delta", delta, "payload_samples", payloadSamples, "clock_rate_hz", clockRate)
	slog.Info("RTP 时间戳与载荷长度不一致",
		"leg_id", legID, "rtp_ts_delta", delta, "payload_samples", payloadSamples, "clock_rate_hz", clockRate)
}
