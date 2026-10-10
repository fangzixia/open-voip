package media

import "testing"

func TestEnterBridgeModeKeepsOneMediaClock(t *testing.T) {
	mix := newScheduledRoomMixer()
	r := &room{mixAudio: true, mixer: mix, direct: false}
	s := &Service{}
	s.enterBridgeMode(r, "a", "b")
	if !r.mixAudio || r.mixer != mix {
		t.Fatal("bridge must retain the room clock")
	}
	if !r.direct || r.bridgeA != "a" || r.bridgeB != "b" {
		t.Fatalf("bridge legs not set: %+v", r)
	}
	select {
	case <-mix.done():
		t.Fatal("bridge stopped the mixer")
	default:
	}
}
