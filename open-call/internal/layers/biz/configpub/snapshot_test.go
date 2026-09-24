package configpub

import "testing"

func TestDIDCandidates(t *testing.T) {
	got := DIDCandidates("+0218001")
	if !contains(got, "0218001") || !contains(got, "8001") {
		t.Fatalf("got %v", got)
	}
	got = DIDCandidates("008613800138000")
	if !contains(got, "8613800138000") {
		t.Fatalf("00/86 got %v", got)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
