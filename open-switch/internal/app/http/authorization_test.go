// 本文件验证authorization的关键行为。
package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"open-switch/internal/ports"
	"testing"
)

type authorizationCalls struct {
	ports.CallControlPort
	ports.SignalingPort
}

func (authorizationCalls) GetCall(context.Context, string) (ports.CallView, error) {
	return ports.CallView{ID: "c"}, nil
}

func TestSwitchTrustsAuthenticatedServiceWithoutPrincipal(t *testing.T) {
	const secret = "internal-test-secret-32"
	h := NewSwitchRouter(SwitchRouterDeps{Applications: testApplicationRegistry(secret), RouterDeps: RouterDeps{CallControl: authorizationCalls{}, Signaling: authorizationCalls{}}})
	cases := []struct {
		token  string
		status int
	}{
		{"", 401},
		{"wrong", 401},
		{secret, 200},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/switch/v2/calls/c", nil)
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Errorf("token %q got %d: %s", tc.token, rec.Code, rec.Body.String())
		}
	}
}

func TestLegacySwitchV1IsRemoved(t *testing.T) {
	deps := SwitchRouterDeps{Applications: testApplicationRegistry("internal-test-secret-32"), RouterDeps: RouterDeps{CallControl: authorizationCalls{}, Signaling: authorizationCalls{}}}
	req := httptest.NewRequest(http.MethodGet, "/switch/v1/calls/c", nil)
	rec := httptest.NewRecorder()
	NewSwitchRouter(deps).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("legacy route status=%d", rec.Code)
	}
}
