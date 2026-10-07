package media

import "sync"

const (
	mixClockRate      = 8000
	defaultPlayoutCap = 4800 * mixInternalRate / mixClockRate // 600 ms @ 16 kHz
)

// legPlayoutBuffer 按 RTP 时间戳重建单腿 PCM（16 kHz），供 20 ms 节拍消费。
type legPlayoutBuffer struct {
	mu sync.Mutex

	ssrc       uint32
	anchorTS   uint32
	anchored   bool
	capSamples int

	playoutSample int64
	baseSample    int64
	buf           []int16

	lastSeq    uint16
	haveSeq    bool
	lastFrame  [mixInternalFrameSamples]int16
	haveLast   bool
	plcUsed    bool
	maxWritten int64
}

func newLegPlayoutBuffer(capSamples int) *legPlayoutBuffer {
	if capSamples <= mixInternalFrameSamples {
		capSamples = defaultPlayoutCap
	}
	return &legPlayoutBuffer{capSamples: capSamples}
}

func (b *legPlayoutBuffer) resetLocked(ssrc, ts uint32) {
	b.ssrc = ssrc
	b.anchorTS = ts
	b.anchored = true
	b.playoutSample = 0
	b.baseSample = 0
	b.buf = nil
	b.haveSeq = false
	b.haveLast = false
	b.plcUsed = false
	b.maxWritten = 0
}

func rtpSampleIndex(ts, anchor uint32) int64 {
	return int64(uint32(ts - anchor))
}

func (b *legPlayoutBuffer) ingest(seq uint16, ssrc, ts uint32, pcm []int16) {
	if len(pcm) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.anchored && ssrc != 0 && b.ssrc != 0 && ssrc != b.ssrc {
		b.resetLocked(ssrc, ts)
	}
	if b.haveSeq && seq == b.lastSeq {
		return
	}
	b.lastSeq = seq
	b.haveSeq = true

	if !b.anchored {
		b.ssrc = ssrc
		b.anchorTS = ts
		b.anchored = true
		b.playoutSample = 0
		b.baseSample = 0
	}
	start := rtpSampleIndex(ts, b.anchorTS)
	b.writeRange(start, pcm)
}

func (b *legPlayoutBuffer) writeRange(start int64, pcm []int16) {
	end := start + int64(len(pcm))
	if end > b.maxWritten {
		b.maxWritten = end
	}
	b.ensureCover(end)
	for i, s := range pcm {
		idx := start + int64(i) - b.baseSample
		if idx < 0 || int(idx) >= len(b.buf) {
			continue
		}
		b.buf[idx] = s
	}
}

func (b *legPlayoutBuffer) ensureCover(end int64) {
	if end <= b.baseSample {
		return
	}
	need := end - b.baseSample
	if int(need) > b.capSamples {
		skip := int(need) - b.capSamples
		b.dropPrefix(int64(skip))
		need = int64(b.capSamples)
	}
	if int(need) > len(b.buf) {
		ext := make([]int16, need)
		copy(ext, b.buf)
		b.buf = ext
	}
}

func (b *legPlayoutBuffer) dropPrefix(n int64) {
	if n <= 0 {
		return
	}
	if n >= int64(len(b.buf)) {
		b.buf = nil
		b.baseSample += n
		return
	}
	b.buf = append([]int16(nil), b.buf[n:]...)
	b.baseSample += n
}

func (b *legPlayoutBuffer) pullFrame() []int16 {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]int16, mixInternalFrameSamples)
	if !b.anchored {
		return out
	}
	start := b.playoutSample
	end := start + mixInternalFrameSamples
	hasData := false
	for i := 0; i < mixInternalFrameSamples; i++ {
		idx := start + int64(i) - b.baseSample
		if idx >= 0 && int(idx) < len(b.buf) {
			out[i] = b.buf[idx]
			if out[i] != 0 {
				hasData = true
			}
		}
	}
	if !hasData && b.haveLast && !b.plcUsed && end <= b.maxWritten+mixInternalFrameSamples {
		for i := 0; i < mixInternalFrameSamples; i++ {
			out[i] = int16(int32(b.lastFrame[i]) * 3 / 4)
		}
		b.plcUsed = true
	} else if hasData {
		b.plcUsed = false
		copy(b.lastFrame[:], out)
		b.haveLast = true
	}
	b.playoutSample += mixInternalFrameSamples
	keepFrom := b.playoutSample - int64(b.capSamples/2)
	if keepFrom > b.baseSample {
		b.dropPrefix(keepFrom - b.baseSample)
	}
	return out
}
