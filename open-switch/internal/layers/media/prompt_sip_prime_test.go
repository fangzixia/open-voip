package media

import (
	"testing"
	"time"
)

func TestSIPWaitInboundUnblocksAfterSignal(t *testing.T) {
	rt := &sipRTP{inboundReady: make(chan struct{})}
	done := make(chan bool, 1)
	go func() {
		done <- rt.waitInbound(2 * time.Second)
	}()
	time.Sleep(20 * time.Millisecond)
	rt.signalInbound()
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("expected inbound wait success")
		}
	case <-time.After(time.Second):
		t.Fatal("wait timed out")
	}
}

func TestSIPWaitInboundTimesOut(t *testing.T) {
	rt := &sipRTP{inboundReady: make(chan struct{})}
	start := time.Now()
	if rt.waitInbound(40 * time.Millisecond) {
		t.Fatal("expected timeout")
	}
	if time.Since(start) < 35*time.Millisecond {
		t.Fatal("wait returned too early")
	}
}
