package media

import (
	"testing"
	"time"
)

func TestPlayoutPreservesLongFrame(t *testing.T) {
	leg := newLegPlayoutBuffer(defaultPlayoutCap)
	long := make([]int16, 240) // 30 ms @ 8 kHz
	for i := range long {
		long[i] = 1000
	}
	leg.ingest(1, 1, 0, long)
	frame := leg.pullFrame()
	if len(frame) != mixInternalFrameSamples {
		t.Fatalf("frame len=%d", len(frame))
	}
	if frame[0] != 1000 || frame[mixInternalFrameSamples-1] != 1000 {
		t.Fatalf("expected first 20ms of long frame, got %d..%d", frame[0], frame[mixInternalFrameSamples-1])
	}
	second := leg.pullFrame()
	if second[0] != 1000 {
		t.Fatalf("second frame should continue long payload, got %d", second[0])
	}
}

func TestPlayoutDeduplicateSeq(t *testing.T) {
	leg := newLegPlayoutBuffer(defaultPlayoutCap)
	a := make([]int16, mixInternalFrameSamples)
	a[0] = 500
	leg.ingest(10, 1, 0, a)
	leg.ingest(10, 1, uint32(mixInternalFrameSamples), a) // duplicate seq
	leg.ingest(11, 1, uint32(mixInternalFrameSamples), func() []int16 {
		b := make([]int16, mixInternalFrameSamples)
		b[0] = 900
		return b
	}())
	f1 := leg.pullFrame()
	f2 := leg.pullFrame()
	if f1[0] != 500 || f2[0] != 900 {
		t.Fatalf("duplicate seq should be ignored: %d %d", f1[0], f2[0])
	}
}

func TestPlayoutSSRCRestart(t *testing.T) {
	leg := newLegPlayoutBuffer(defaultPlayoutCap)
	a := make([]int16, mixInternalFrameSamples)
	a[0] = 111
	leg.ingest(1, 1, 0, a)
	leg.ingest(1, 2, 0, a) // new SSRC
	f := leg.pullFrame()
	if f[0] != 111 {
		t.Fatalf("after ssrc restart expected fresh anchor, got %d", f[0])
	}
}

func TestRecordingTimelineNoOverlapWrite(t *testing.T) {
	m := newPCMMix(8000, time.Now())
	m.writeLinearAtSampleIdx(0, []int16{100, 200}, 8000)
	m.writeLinearAtSampleIdx(0, []int16{300, 400}, 8000)
	if m.samples[0] != 300 || m.samples[1] != 400 {
		t.Fatalf("leg track should overwrite not add: %v", m.samples[:2])
	}
}
