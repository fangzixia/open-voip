package media

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestDeferUntilAnsweredRunsAfterRelease(t *testing.T) {
	s := &Service{answers: answerGate{}}
	s.markSIPAnswerPending("c1")
	var ran atomic.Bool
	if !s.DeferUntilAnswered("c1", func() { ran.Store(true) }) {
		t.Fatal("expected defer while pending")
	}
	if ran.Load() {
		t.Fatal("must not run before answer")
	}
	s.finishSIPAnswer("c1", true)
	deadline := time.Now().Add(2 * time.Second)
	for !ran.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !ran.Load() {
		t.Fatal("expected callback after 200 OK")
	}
}

func TestDeferUntilAnsweredDroppedOnFailure(t *testing.T) {
	s := &Service{answers: answerGate{}}
	s.markSIPAnswerPending("c2")
	var ran atomic.Bool
	s.DeferUntilAnswered("c2", func() { ran.Store(true) })
	s.finishSIPAnswer("c2", false)
	if ran.Load() {
		t.Fatal("callback must be dropped when call fails before answer")
	}
}
