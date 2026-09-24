package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimitRejectsRequestsOverLimit(t *testing.T) {
	handler := RateLimit(2)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for i, want := range []int{http.StatusNoContent, http.StatusNoContent, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("request %d: got status %d, want %d", i+1, res.Code, want)
		}
	}
}

func TestRateLimitSeparatesSources(t *testing.T) {
	handler := RateLimit(1)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, remoteAddr := range []string{"192.0.2.1:1234", "192.0.2.2:1234"} {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = remoteAddr
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusNoContent {
			t.Fatalf("source %s: got status %d", remoteAddr, res.Code)
		}
	}
}
