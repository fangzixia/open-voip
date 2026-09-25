package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"open-switch/internal/config"
	"open-switch/internal/httpapi"
	"testing"
)

func TestClientUnifiedRoundTrip(t *testing.T) {
	peer := httptest.NewServer(httpapi.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer integration-secret" {
			t.Error("missing authentication")
		}
		if r.URL.Path == "/test" && r.Header.Get("X-Request-ID") != "round-trip" {
			t.Error("missing trace")
		}
		if r.URL.Path == "/test" && r.Header.Get("X-Trace-ID") != "trace-round-trip" {
			t.Error("missing trace id")
		}
		httpapi.Write(w, 200, map[string]string{"id": "call-1"})
	})))
	defer peer.Close()
	client := NewClient(config.IntegrationConfig{PlatformBaseURL: peer.URL, Secret: "integration-secret"})
	var out struct {
		ID string `json:"id"`
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "round-trip")
	req.Header.Set("X-Trace-ID", "trace-round-trip")
	httpapi.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := client.doJSON(r.Context(), "GET", "/test", nil, &out); err != nil {
			t.Fatal(err)
		}
	})).ServeHTTP(httptest.NewRecorder(), req)
	if out.ID != "call-1" {
		t.Fatalf("response not unwrapped: %+v", out)
	}
	if err := client.doJSON(context.Background(), "GET", "/empty", nil, nil); err != nil {
		t.Fatal(err)
	}
}
