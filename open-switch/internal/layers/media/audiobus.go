package media

import "sync"

// audioBus 以 48 kHz 浮点 PCM 作为混音工作格式，桥接不同编解码腿。
type audioBus struct {
	mu      sync.Mutex
	rate    int
	samples []float32
	legIdx  map[string]int
	started int64
}

const busRate = 48000

func newAudioBus() *audioBus {
	return &audioBus{rate: busRate, legIdx: map[string]int{}}
}

func (b *audioBus) ingestLeg(legID string, pcm []int16, fromRate int) {
	if b == nil || legID == "" || len(pcm) == 0 {
		return
	}
	b.mu.Lock()
	idx := b.legIdx[legID]
	b.mu.Unlock()
	b.mixPCM16At(idx, pcm, fromRate)
	b.mu.Lock()
	b.legIdx[legID] = idx + len(pcm)*busRate/fromRate
	b.mu.Unlock()
}

func (b *audioBus) mixPCM16At(idx int, pcm []int16, fromRate int) {
	if b == nil || len(pcm) == 0 {
		return
	}
	if fromRate <= 0 {
		fromRate = 8000
	}
	for i, s := range pcm {
		srcIdx := idx + i*busRate/fromRate
		need := srcIdx + 1
		if need > len(b.samples) {
			ext := make([]float32, need)
			copy(ext, b.samples)
			b.samples = ext
		}
		b.samples[srcIdx] += float32(s) / 32768
	}
}

func (b *audioBus) renderPCM16(fromIdx, count int, toRate int) []int16 {
	if b == nil || count <= 0 || toRate <= 0 {
		return nil
	}
	out := make([]int16, count*toRate/busRate)
	for i := range out {
		src := fromIdx + i*busRate/toRate
		if src >= len(b.samples) {
			break
		}
		v := b.samples[src] * 32767
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		out[i] = int16(v)
	}
	return out
}
