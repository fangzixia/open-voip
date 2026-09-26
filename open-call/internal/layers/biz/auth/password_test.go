// 本文件验证password的关键行为。
package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	h, err := HashPassword("StrongPass123")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassword("StrongPass123", h); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassword("wrong", h); err == nil {
		t.Fatal("expected mismatch")
	}
}
