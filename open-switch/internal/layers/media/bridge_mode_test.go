package media

import "testing"

func TestEnterBridgeModeStopsMix(t *testing.T) {
	mix := newScheduledRoomMixer(nil)
	r := &room{mixAudio: true, mixer: mix, direct: false}
	s := &Service{}
	s.enterBridgeMode(r, "a", "b")
	if r.mixAudio {
		t.Fatal("mixAudio should be false after bridge")
	}
	if !r.direct || r.bridgeA != "a" || r.bridgeB != "b" {
		t.Fatalf("bridge legs not set: %+v", r)
	}
	select {
	case <-mix.done():
	default:
		t.Fatal("mixer should be stopped")
	}
}
