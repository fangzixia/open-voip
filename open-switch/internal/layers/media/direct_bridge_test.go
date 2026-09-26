package media

import "testing"

func TestDirectRoomOnlyForwardsAcrossBridge(t *testing.T) {
	r := &room{direct: true}
	if r.canForward("a", "b") {
		t.Fatal("media flowed before bridge")
	}
	r.bridgeA, r.bridgeB = "a", "b"
	for _, pair := range [][2]string{{"a", "b"}, {"b", "a"}} {
		if !r.canForward(pair[0], pair[1]) {
			t.Fatalf("bridge blocked %v", pair)
		}
	}
	if r.canForward("a", "c") || r.canForward("c", "b") {
		t.Fatal("media escaped the two-leg bridge")
	}
}
