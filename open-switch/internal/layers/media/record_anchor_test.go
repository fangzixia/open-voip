package media

import (
	"testing"
	"time"
)

func TestPCMAnchorStartsPromptAtZero(t *testing.T) {
	t0 := time.Now().Add(-2 * time.Second)
	m := newPCMMix(8000, t0)
	m.anchorAt(time.Now())
	m.add(0, mulawToneFrame())
	if len(m.samples) == 0 {
		t.Fatal("expected samples")
	}
	if len(m.samples) > 200 {
		t.Fatalf("anchored prompt should start near 0, len=%d", len(m.samples))
	}
}
