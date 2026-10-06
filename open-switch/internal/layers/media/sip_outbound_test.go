package media

import (
	"testing"
)

func TestSipInviteRejectMessage(t *testing.T) {
	msg, code := sipInviteRejectMessage(486)
	if code != "SIP_BUSY" || msg == "" {
		t.Fatalf("486: got %q %q", msg, code)
	}
	msg, code = sipInviteRejectMessage(480)
	if code != "SIP_UNAVAILABLE" {
		t.Fatalf("480: got %q", code)
	}
	msg, code = sipInviteRejectMessage(503)
	if code != "SIP_REJECT" {
		t.Fatalf("503: got %q", code)
	}
}
