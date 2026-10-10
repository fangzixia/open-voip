package media

import (
	"encoding/binary"
	"errors"
	"github.com/livekit/media-sdk/jitter"
	"github.com/pion/rtp"
	"sync"
	"time"
)

const (
	mixClockRate      = 8000
	defaultPlayoutCap = 1600
	playoutReserve    = 60 * time.Millisecond
	// Variable soxr contributes 30 ms native delay. Its conversion is primed
	// once, and the PCM reserve shares the total 60 ms delay budget.
	initialReorderReserve = 20 * time.Millisecond
	inputPCMReserve       = 30 * time.Millisecond
)

type audioDepacketizer struct{}

func (audioDepacketizer) Unmarshal(p []byte) ([]byte, error) { return p, nil }
func (audioDepacketizer) IsPartitionHead([]byte) bool        { return true }
func (audioDepacketizer) IsPartitionTail(bool, []byte) bool  { return true }

// The adapter only owns sample positions and lifecycle. Ordering is LiveKit;
// presence is independent of value, and all concealment is SpanDSP.
type legPlayoutBuffer struct {
	generation                               uint64
	resampler                                *streamingResampler
	clockPPM, clockFraction, filteredError   float64
	lastCorrection                           time.Time
	nativeError                              error
	outputPending                            []int16
	now                                      func() time.Time
	mu                                       sync.Mutex
	jitter                                   *jitter.Buffer
	plc                                      *g711PLC
	capSamples                               int
	readyAt                                  time.Time
	anchored                                 bool
	lastTS                                   uint32
	ssrc                                     uint32
	position, cursor, maxWritten             int64
	samples                                  map[int64]int16
	closed                                   bool
	switches, restarts                       uint64
	lateRunSamples                           int
	resyncs                                  uint64
	lateSamples, plcSamples, overflowSamples uint64
}

func newLegPlayoutBuffer(capSamples int) *legPlayoutBuffer {
	if capSamples <= 3*mixFrameSamples {
		capSamples = defaultPlayoutCap
	}
	b := &legPlayoutBuffer{now: time.Now, capSamples: capSamples, samples: map[int64]int16{}, plc: newG711PLC()}
	b.resampler, b.nativeError = newStreamingResampler(8000, 8000, true)
	b.primeResamplerLocked()
	b.jitter = jitter.NewBuffer(audioDepacketizer{}, playoutReserve, b.accept, jitter.WithBatchDelivery(), jitter.WithStartupDelay(initialReorderReserve), jitter.WithSequenceRestartDetection(), jitter.WithStatsHandler(func(s *jitter.BufferStats) {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.closed {
			return
		}
		if s.SSRCSwitches != b.switches || s.SequenceRestarts != b.restarts {
			b.resetLocked()
			b.switches, b.restarts = s.SSRCSwitches, s.SequenceRestarts
		}
	}))
	return b
}
func rtpSampleIndex(ts, anchor uint32) int64 { return int64(int32(ts - anchor)) }
func (b *legPlayoutBuffer) resetLocked() {
	b.generation++
	b.anchored = false
	b.position = 0
	b.cursor = 0
	b.maxWritten = 0
	b.readyAt = b.now().Add(inputPCMReserve)
	clear(b.samples)
	b.plc.close()
	b.plc = newG711PLC()
	b.resampler.close()
	b.resampler, b.nativeError = newStreamingResampler(8000, 8000, true)
	b.clockPPM, b.clockFraction, b.filteredError = 0, 0, 0
	b.primeResamplerLocked()
	b.lastCorrection = time.Time{}
	b.lateRunSamples = 0
}
func (b *legPlayoutBuffer) ingest(seq uint16, ssrc, ts uint32, pcm []int16) {
	if len(pcm) == 0 {
		return
	}
	b.push(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: ts, SSRC: ssrc, PayloadType: 127}, Payload: encodePCM16(pcm)})
}
func (b *legPlayoutBuffer) push(pkt *rtp.Packet) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	if b.readyAt.IsZero() {
		b.readyAt = b.now().Add(inputPCMReserve)
	}
	b.mu.Unlock()
	b.jitter.Push(pkt.Clone())
}
func (b *legPlayoutBuffer) accept(packets []jitter.ExtPacket) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for _, ext := range packets {
		p := ext.Packet
		b.ssrc = p.SSRC
		var pcm []int16
		switch p.PayloadType {
		case 0:
			pcm = pcmuPayloadToPCM(p.Payload)
		case 8:
			pcm = make([]int16, len(p.Payload))
			for i, v := range p.Payload {
				pcm[i] = alawToLinear(v)
			}
		case 127:
			pcm = make([]int16, len(p.Payload)/2)
			for i := range pcm {
				pcm[i] = int16(binary.LittleEndian.Uint16(p.Payload[2*i:]))
			}
		default:
			continue
		}
		if !b.anchored {
			b.anchored = true
			b.lastTS = p.Timestamp
			b.position = 0
		} else {
			b.position += rtpSampleIndex(p.Timestamp, b.lastTS)
			b.lastTS = p.Timestamp
		}
		start := b.position
		end := start + int64(len(pcm))
		// A persistent latency step can leave every subsequent ordered packet
		// behind the cursor. Rate correction handles clock drift, not a new
		// network phase. Discard expired audio and re-anchor future playback.
		if end <= b.cursor && start >= b.maxWritten {
			b.lateRunSamples += len(pcm)
		} else {
			b.lateRunSamples = 0
		}
		if b.lateRunSamples >= 3*mixFrameSamples {
			b.lateSamples += uint64(len(pcm))
			b.maxWritten = end
			b.cursor = end
			clear(b.samples)
			b.plc.close()
			b.plc = newG711PLC()
			b.resampler.close()
			b.resampler, b.nativeError = newStreamingResampler(8000, 8000, true)
			b.clockPPM, b.clockFraction, b.filteredError = 0, 0, 0
			b.lastCorrection = time.Time{}
			b.primeResamplerLocked()
			b.readyAt = b.now().Add(inputPCMReserve)
			b.generation++
			b.resyncs++
			b.lateRunSamples = 0
			continue
		}
		if end-b.cursor > int64(b.capSamples-320) {
			newCursor := end - int64(b.capSamples-320)
			b.overflowSamples += uint64(newCursor - b.cursor)
			b.cursor = newCursor
			for pos := range b.samples {
				if pos < newCursor {
					delete(b.samples, pos)
				}
			}
			b.plc.close()
			b.plc = newG711PLC()
			b.generation++
			b.resampler.close()
			b.resampler, b.nativeError = newStreamingResampler(8000, 8000, true)
			b.clockFraction = 0
			b.primeResamplerLocked()
		}
		b.maxWritten = max(b.maxWritten, end)
		for i, v := range pcm {
			pos := start + int64(i)
			if pos < b.cursor {
				b.lateSamples++
				continue
			}
			b.samples[pos] = v
		}
	}
}
func (b *legPlayoutBuffer) pullFrame() []int16 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pullSamplesLocked(mixFrameSamples)
}

func (b *legPlayoutBuffer) pullSamplesLocked(n int) []int16 {
	out := make([]int16, n)
	if b.closed || !b.anchored || b.now().Before(b.readyAt) {
		return out
	}
	for offset := 0; offset < len(out); {
		_, present := b.samples[b.cursor+int64(offset)]
		end := offset
		for end < len(out) {
			pos := b.cursor + int64(end)
			v, ok := b.samples[pos]
			if ok != present {
				break
			}
			out[end] = v
			delete(b.samples, pos)
			end++
		}
		if present {
			b.plc.receive(out[offset:end])
		} else if b.cursor+int64(offset) < b.maxWritten+int64(defaultPlayoutCap) {
			b.plc.fill(out[offset:end])
			b.plcSamples += uint64(end - offset)
		}
		offset = end
	}
	b.cursor += int64(len(out))
	return out
}

func (b *legPlayoutBuffer) pullAdaptiveFrame() []int16 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.nativeError != nil {
		return nil
	}
	if !b.anchored || b.now().Before(b.readyAt) {
		return make([]int16, mixFrameSamples)
	}
	now := b.now()
	if b.lastCorrection.IsZero() {
		b.lastCorrection = now
	} else if now.Sub(b.lastCorrection) >= time.Second {
		errorSamples := float64(b.maxWritten-b.cursor) - float64(3*mixFrameSamples)
		b.filteredError = .9*b.filteredError + .1*errorSamples
		// A proportional occupancy controller avoids integrating packet-size
		// steps into a permanent rate error. 2 ppm per sample gives about a
		// one-minute settling time; the 10-second filter rejects arrival jitter.
		targetPPM := max(-300.0, min(300.0, b.filteredError*2))
		b.clockPPM += max(-25.0, min(25.0, targetPPM-b.clockPPM))
		b.nativeError = b.resampler.setPPM(b.clockPPM)
		b.lastCorrection = now
	}
	b.clockFraction += b.clockPPM * float64(mixFrameSamples) / 1e6
	extra := int(b.clockFraction)
	b.clockFraction -= float64(extra)
	pcm := b.pullSamplesLocked(mixFrameSamples + extra)
	out, e := b.resampler.process(pcm, false)
	if e != nil {
		b.nativeError = e
		return nil
	}
	b.outputPending = append(b.outputPending, out...)
	if len(b.outputPending) < mixFrameSamples {
		return nil
	}
	frame := append([]int16(nil), b.outputPending[:mixFrameSamples]...)
	b.outputPending = append(b.outputPending[:0], b.outputPending[mixFrameSamples:]...)
	return frame
}
func (b *legPlayoutBuffer) close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.jitter.Close()
	b.mu.Lock()
	b.plc.close()
	b.resampler.close()
	clear(b.samples)
	b.mu.Unlock()
}

func (b *legPlayoutBuffer) primeResamplerLocked() {
	// Warm the native converter only when a stream/epoch is created. This is
	// not a per-frame flush. A one-millisecond elastic margin absorbs the
	// fractional sample changes while libsoxr slews its ratio.
	b.outputPending = make([]int16, 8)
	if b.plc == nil || b.plc.ptr == nil {
		b.nativeError = errors.New("SpanDSP plc_init failed")
	}
	if b.nativeError == nil {
		_, b.nativeError = b.resampler.process(make([]int16, 480), false)
	}
}
