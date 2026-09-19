package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"open-voip/internal/config"
)

func TestHealthEndpoint(t *testing.T) {
	h := NewRouter(RouterDeps{
		Config: config.Config{},
		Status: StatusProvider{},
	})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
