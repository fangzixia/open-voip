package http

import (
	"context"
	"net/http/httptest"
	"open-switch/internal/config"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"strings"
	"testing"
)

type authorizationCalls struct {
	ports.CallControlPort
	ports.SignalingPort
}

func (authorizationCalls) GetCall(context.Context, string) (ports.CallView, error) {
	return ports.CallView{ID: "c", State: "queued", AgentID: "owner", Legs: []ports.LegView{{ID: "customer", Role: dto.LegRoleCustomer}, {ID: "agent", Role: dto.LegRoleAgent, AgentID: "owner"}}}, nil
}

func TestSwitchRejectsUnrelatedPrincipalAndLeg(t *testing.T) {
	h := NewSwitchRouter(SwitchRouterDeps{Config: config.Config{Integration: config.IntegrationConfig{Secret: "internal"}}, RouterDeps: RouterDeps{CallControl: authorizationCalls{}, Signaling: authorizationCalls{}}})
	cases := []struct {
		path, principal string
		status          int
	}{
		{"/switch/v1/calls/c", `{"agent_id":"other","role":"agent"}`, 403},
		{"/switch/v1/calls/c", `{"guest_id":"g","role":"guest"}`, 403},
		{"/switch/v1/calls/c", `{"guest_id":"g","role":"guest","guest_call_id":"c"}`, 200},
		{"/switch/v1/calls/c/legs/customer/mute", `{"agent_id":"owner","role":"agent"}`, 403},
		{"/switch/v1/supervisor/agents/owner/force-check-out", `{"agent_id":"owner","role":"agent"}`, 403},
	}
	for _, tc := range cases {
		method := "POST"
		if tc.path == "/switch/v1/calls/c" {
			method = "GET"
		}
		req := httptest.NewRequest(method, tc.path, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer internal")
		req.Header.Set("X-Principal", tc.principal)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Errorf("%s got %d: %s", tc.path, rec.Code, rec.Body.String())
		}
	}
}
