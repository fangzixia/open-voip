// 本文件验证record wav的关键行为。
package media

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMulawRoundTrip(t *testing.T) {
	in := int16(8000)
	got := mulawToLinear(linearToMulaw(in))
	if got == 0 {
		t.Fatalf("μ-law roundtrip collapsed to 0")
	}
	if abs16(got-in) > 500 {
		t.Fatalf("μ-law roundtrip too far: in=%d out=%d", in, got)
	}
}

func TestMulawKnownValues(t *testing.T) {
	if got := linearToMulaw(0); got != 0xff {
		t.Fatalf("silence must be PCMU 0xff, got %#x", got)
	}
	for _, in := range []int16{1, 100, 1400, 8000, 30000, -1, -100, -1400, -8000, -30000, -32768} {
		got := int(mulawToLinear(linearToMulaw(in)))
		if diff := max(got-int(in), int(in)-got); diff > 1500 {
			t.Fatalf("PCMU distortion: in=%d out=%d", in, got)
		}
	}
}

func TestFallbackToneHasNoHarshSquareWave(t *testing.T) {
	frame := mulawToneFrame()
	if len(frame) != 160 {
		t.Fatalf("tone frame length=%d", len(frame))
	}
	peak, maxStep := 0, 0
	previous := int(mulawToLinear(frame[len(frame)-1]))
	for _, sample := range frame {
		value := int(mulawToLinear(sample))
		if abs := max(value, -value); abs > peak {
			peak = abs
		}
		if step := max(value-previous, previous-value); step > maxStep {
			maxStep = step
		}
		previous = value
	}
	if peak < 500 || peak > 3000 || maxStep > 1000 {
		t.Fatalf("harsh fallback tone: peak=%d max_step=%d", peak, maxStep)
	}
}

func TestPCMMixWritesPlayableWAV(t *testing.T) {
	m := newPCMMix(8000, time.Now().Add(-20*time.Millisecond))
	frame := mulawToneFrame()
	m.add(0, frame)
	if len(m.samples) < len(frame) {
		t.Fatalf("samples=%d want >= %d", len(m.samples), len(frame))
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "rec.wav")
	n, err := m.writeWAV(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(b)) != n || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		t.Fatalf("bad wav header n=%d len=%d magic=%q", n, len(b), b[:12])
	}
	if binary.LittleEndian.Uint16(b[20:22]) != 1 || binary.LittleEndian.Uint32(b[24:28]) != 8000 {
		t.Fatalf("wav fmt pcm/rate mismatch")
	}
}

func abs16(v int16) int16 {
	if v < 0 {
		return -v
	}
	return v
}

func TestStreamingRecordingHasBoundedMemoryAndValidHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.wav")
	m := newPCMMix(8000, time.Now())
	if err := m.startFile(path); err != nil {
		t.Fatal(err)
	}
	for second := 0; second < 120; second++ {
		m.started = time.Now().Add(-time.Duration(second) * time.Second)
		m.add(0, mulawToneFrame())
		if len(m.samples) > m.rate+200 {
			t.Fatalf("unbounded buffer: %d", len(m.samples))
		}
	}
	size, err := m.writeWAV(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(b)) != size || int64(binary.LittleEndian.Uint32(b[40:])) != size-44 {
		t.Fatal("invalid finalized header")
	}
}
