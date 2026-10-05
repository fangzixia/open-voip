package media

import "testing"

func TestSIPOutboundLooksPSTN(t *testing.T) {
	if !sipOutboundLooksPSTN("13800138000") {
		t.Fatal("mobile")
	}
	if sipOutboundLooksPSTN("8001") {
		t.Fatal("short extension")
	}
	if !sipOutboundLooksPSTN("+8613800138000") {
		t.Fatal("e164")
	}
}
