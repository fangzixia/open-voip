package realtime

import "testing"

func TestResamplePCM24kTo8kLength(t *testing.T) {
	in := make([]int16, 2400)
	for i := range in {
		in[i] = int16(1000 * (i % 40))
	}
	out := ResamplePCM(in, 24000, 8000)
	if len(out) != 800 {
		t.Fatalf("expected 800 samples, got %d", len(out))
	}
}

func TestResamplePCM8kTo16kDoubles(t *testing.T) {
	in := []int16{0, 1000, 2000, 3000}
	out := ResamplePCM(in, 8000, 16000)
	if len(out) != 8 {
		t.Fatalf("expected 8 samples, got %d", len(out))
	}
}

func TestResampleSimpleAlias(t *testing.T) {
	in := []int16{0, 100, 200, 300}
	if len(ResampleSimple(in, 8000, 16000)) != 8 {
		t.Fatal("ResampleSimple should match ResamplePCM")
	}
}
