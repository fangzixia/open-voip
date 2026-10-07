package media

import "sync"

type recordLegTimeline struct {
	clockRate int
	anchorTS  uint32
	anchored  bool
	ssrc      uint32
}

type recordTimelineStore struct {
	mu    sync.Mutex
	legs  map[string]*recordLegTimeline
}

func newRecordTimelineStore() *recordTimelineStore {
	return &recordTimelineStore{legs: map[string]*recordLegTimeline{}}
}

func (s *recordTimelineStore) sampleIndex(legID string, ssrc, ts uint32, nSamples int, clockRate int) int64 {
	if s == nil || nSamples <= 0 || clockRate <= 0 {
		return -1
	}
	if clockRate != 8000 && clockRate != 16000 && clockRate != 48000 {
		clockRate = 8000
	}
	s.mu.Lock()
	t := s.legs[legID]
	if t == nil {
		t = &recordLegTimeline{clockRate: clockRate}
		s.legs[legID] = t
	}
	if t.anchored && ssrc != 0 && t.ssrc != 0 && ssrc != t.ssrc {
		t.anchorTS = ts
		t.ssrc = ssrc
		t.clockRate = clockRate
	}
	if !t.anchored {
		t.anchorTS = ts
		t.ssrc = ssrc
		t.anchored = true
		t.clockRate = clockRate
	}
	idx := rtpSampleIndex(ts, t.anchorTS)
	s.mu.Unlock()
	return idx
}
