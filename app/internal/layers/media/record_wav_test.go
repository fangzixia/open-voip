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
