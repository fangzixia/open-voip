// 本文件验证cors的关键行为。
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAllowsPreflight(t *testing.T) {
	h := CORS([]string{"https://ui.example.com"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "https://ui.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://ui.example.com" {
		t.Fatalf("allow-origin=%q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSRejectsUnlistedPreflight(t *testing.T) {
	h := CORS([]string{"https://ui.example.com"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/status", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("allow-origin=%q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}
