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

func TestScheduledMixerExcludesSelf(t *testing.T) {
	m := newScheduledRoomMixer(nil)
	pcmA := make([]int16, mixFrameSamples)
	pcmB := make([]int16, mixFrameSamples)
	pcmA[0], pcmB[0] = 8000, -4000
	m.ingest("a", 1, 0, 1, 8000, pcmA)
	m.ingest("b", 1, 0, 2, 8000, pcmB)
	frames := m.advanceTick()
	mixA := mixPCMFrames(frames, "a")
	if len(mixA) < 1 || mixA[0] >= 0 {
		t.Fatalf("a should only hear b, got %v", mixA)
	}
}
