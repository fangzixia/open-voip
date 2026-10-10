package media

import (
	"testing"
	"time"
)

func TestSlowDestinationCannotBlockAnotherDestination(t *testing.T) {
	blocked, release := make(chan struct{}), make(chan struct{})
	slow := newMediaOutput(func([]byte) {
		select {
		case <-blocked:
		default:
			close(blocked)
		}
		<-release
	})
	defer slow.stop()
	ack := make(chan byte, 1)
	fast := newMediaOutput(func(p []byte) { ack <- p[0] })
	defer fast.stop()
	slow.enqueue([]byte{0})
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("slow sender not reached")
	}
	defer close(release)
	for i := range 100 {
		slow.enqueue([]byte{byte(i)})
		fast.enqueue([]byte{byte(i)})
		select {
		case got := <-ack:
			if got != byte(i) {
				t.Fatal("healthy destination reordered")
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("slow destination stalled healthy destination")
		}
	}
	slow.mu.Lock()
	n, drops := len(slow.queue), slow.dropped
	slow.mu.Unlock()
	if n != 10 || drops != 90 {
		t.Fatalf("unbounded/stale queue: %d, dropped %d", n, drops)
	}
}
