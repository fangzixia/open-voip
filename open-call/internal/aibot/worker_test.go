package aibot

import (
	"testing"

	"open-call/internal/errs"
)

func TestIsBusyCheckIn(t *testing.T) {
	if isBusyCheckIn(errs.Conflict("振铃或通话中不能重新签入", errs.CodeAgentBusy)) {
		// ok
	} else {
		t.Fatal("expected busy check-in")
	}
	if isBusyCheckIn(errs.Conflict("other", "")) {
		t.Fatal("unexpected busy")
	}
}
