package media

import "testing"

func TestDownsample16kTo8kHalvesLength(t *testing.T) {
	in := make([]int16, 320)
	for i := range in {
		in[i] = int16(i * 100)
	}
	out, err := resamplePCM(in, 16000, 8000)
	if err != nil {
		t.Fatal(err)
	}
	rate := 8000
	if rate != 8000 || len(out) != 160 {
		t.Fatalf("rate=%d len=%d", rate, len(out))
	}
	if out[0] == 0 {
		t.Fatalf("unexpected first sample %d", out[0])
	}
}
