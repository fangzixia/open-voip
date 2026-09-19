package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	h, err := HashPassword("changeme")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassword("changeme", h); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassword("wrong", h); err == nil {
		t.Fatal("expected mismatch")
	}
}
