package media

import "testing"

func TestRtpPayloadToPCMUG711(t *testing.T) {
	in := []byte{0xff, 0x7f}
	out := rtpPayloadToPCMU(0, in)
	if len(out) != 2 || out[0] != 0xff {
		t.Fatalf("pcmu passthrough: %v", out)
	}
	out8 := rtpPayloadToPCMU(8, in)
	if len(out8) != 2 {
		t.Fatal("pcma transcode")
	}
	if rtpPayloadToPCMU(99, in) != nil {
		t.Fatal("unknown PT")
	}
}

func TestRoomMixerExcludesSelf(t *testing.T) {
	m := newRoomMixer()
	m.ingest("a", pcmToPCMU([]int16{8000, 8000}))
	m.ingest("b", pcmToPCMU([]int16{-4000, -4000}))
	mixA := m.mixExcept("a")
	if len(mixA) < 2 || mixA[0] >= 0 {
		t.Fatalf("a should only hear b, got %v", mixA)
	}
}
