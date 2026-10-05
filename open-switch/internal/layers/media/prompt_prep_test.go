package media

import "testing"

func TestPreparePromptPCMTrimsLeadingSilence(t *testing.T) {
	pcm := make([]int16, 400)
	for i := 200; i < len(pcm); i++ {
		pcm[i] = 5000
	}
	out := preparePromptPCM(pcm)
	if len(out) >= 400 {
		t.Fatalf("expected trim, len=%d", len(out))
	}
	if len(out) < 50 || out[len(out)-1] < 1000 {
		t.Fatal("expected speech content after trim")
	}
}

func TestPreparePromptPCMKeepsSoftOnset(t *testing.T) {
	pcm := make([]int16, 800)
	for i := 180; i < 220; i++ {
		pcm[i] = 250
	}
	for i := 220; i < len(pcm); i++ {
		pcm[i] = 5000
	}
	out := preparePromptPCM(pcm)
	if len(out) < 600 {
		t.Fatalf("soft onset trimmed too much, len=%d", len(out))
	}
	var peak int
	for i := 0; i < 240 && i < len(out); i++ {
		v := int(out[i])
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
	}
	if peak < 80 {
		t.Fatalf("soft onset region too quiet, peak=%d", peak)
	}
}

func TestPCMToG711PCMA(t *testing.T) {
	pcm := []int16{0, 1000, -2000}
	a := pcmToG711(pcm, 8)
	u := pcmToG711(pcm, 0)
	if len(a) != 3 || len(u) != 3 || a[1] == u[1] {
		t.Fatal("PCMA and PCMU encodings should differ for non-silence")
	}
}
