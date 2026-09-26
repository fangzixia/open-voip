// 本文件验证authorization的关键行为。
package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"open-switch/internal/config"
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
	h := NewSwitchRouter(SwitchRouterDeps{Config: config.Config{Integration: config.IntegrationConfig{Secret: "internal"}}, RouterDeps: RouterDeps{CallControl: authorizationCalls{}, Signaling: authorizationCalls{}}})
	cases := []struct {
		token  string
		status int
	}{
		{"", 401},
		{"wrong", 401},
		{"internal", 200},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/switch/v1/calls/c", nil)
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

type routeOnlyDirect struct{ ports.DirectControlPort }

func TestSwitchModeRoutesAreSeparated(t *testing.T) {
	base := SwitchRouterDeps{Config: config.Config{Integration: config.IntegrationConfig{Secret: "internal"}}, RouterDeps: RouterDeps{CallControl: authorizationCalls{}, Signaling: authorizationCalls{}}}
	for _, tc := range []struct {
		direct bool
		path   string
	}{
		{true, "/switch/v1/calls/outbound"},
		{true, "/switch/v1/calls/inbound"},
		{true, "/switch/v1/supervisor/calls/c/listen"},
		{false, "/switch/v1/calls/direct"},
	} {
		deps := base
		if tc.direct {
			deps.Direct = routeOnlyDirect{}
		}
		req := httptest.NewRequest(http.MethodPost, tc.path, nil)
		req.Header.Set("Authorization", "Bearer internal")
		rec := httptest.NewRecorder()
		NewSwitchRouter(deps).ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("direct=%v path=%s status=%d", tc.direct, tc.path, rec.Code)
		}
	}
}
