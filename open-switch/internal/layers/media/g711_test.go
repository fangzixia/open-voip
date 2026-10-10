// 本文件验证record wav的关键行为。
package media

import (
	"testing"
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

func abs16(v int16) int16 {
	if v < 0 {
		return -v
	}
	return v
}
