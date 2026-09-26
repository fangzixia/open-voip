// 本文件验证bff的关键行为。
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"open-call/internal/config"
	"open-call/internal/errs"
	"open-call/internal/httpapi"
	"open-call/internal/layers/biz/auth"
	"open-call/internal/ports"
	"open-call/internal/ports/dto"
	"strings"
	"testing"
)

type bffAuth struct{}

func (bffAuth) Authenticate(_ context.Context, token string) (auth.Principal, error) {
	if token == "readonly" {
		return auth.Principal{UserID: "viewer", Role: "custom", Permissions: []string{"calls.read"}}, nil
	}
	if token != "valid" {
		return auth.Principal{}, errs.Unauthorized("invalid")
	}
	return auth.Principal{UserID: "user", AgentID: "seat", Role: "agent", Permissions: []string{"calls.operate"}}, nil
}

func TestBFFChecksCallAndLegOwnershipAndSetsActor(t *testing.T) {
	forwarded := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/switch/v1/internal/calls/") {
			id := strings.TrimPrefix(r.URL.Path, "/switch/v1/internal/calls/")
			view := ports.CallView{ID: id, AgentID: "other", Legs: []ports.LegView{
				{ID: "own-leg", Role: dto.LegRoleAgent, AgentID: "seat"},
				{ID: "foreign-leg", Role: dto.LegRoleAgent, AgentID: "other"},
			}}
			if id == "foreign" {
				view.Legs = nil
			}
			httpapi.Write(w, http.StatusOK, view)
			return
		}
		forwarded++
		if r.URL.Path == "/switch/v1/calls/outbound" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["agent_id"] != "seat" {
				t.Errorf("outbound actor not replaced: %v %v", body, err)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	h := WrapSwitchBFF(config.IntegrationConfig{Secret: "internal-secret", SwitchBaseURL: upstream.URL}, bffAuth{}, http.NotFoundHandler())
	cases := []struct {
		path, body string
		want       int
	}{
		{"/api/v1/calls/foreign/hangup", `{}`, http.StatusForbidden},
		{"/api/v1/calls/owned/legs/foreign-leg/offer", `{}`, http.StatusForbidden},
		{"/api/v1/calls/owned/legs/own-leg/offer", `{}`, http.StatusNoContent},
		{"/api/v1/calls/outbound", `{"agent_id":"other"}`, http.StatusNoContent},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer valid")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s: got %d, want %d: %s", tc.path, rec.Code, tc.want, rec.Body.String())
		}
	}
	if forwarded != 2 {
		t.Fatalf("forwarded %d requests, want 2", forwarded)
	}
}

func TestBFFAuthenticatesAndReplacesForgedIdentity(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer internal-secret" {
			t.Error("missing integration secret")
		}
		if r.Header.Get("X-Principal") != "" {
			t.Error("end-user principal must not reach switch")
		}
		if r.URL.Path == "/switch/v1/internal/calls/call" {
			httpapi.Write(w, http.StatusOK, ports.CallView{ID: "call", AgentID: "seat"})
			return
		}
		requests++
		if r.Header.Get("X-Trace-ID") == "" || r.Header.Get("X-Call-ID") != "call" || r.Header.Get("X-Agent-ID") != "seat" {
			t.Errorf("trace headers missing: %v", r.Header)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	local := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) })
	h := WrapSwitchBFF(config.IntegrationConfig{Secret: "internal-secret", SwitchBaseURL: upstream.URL}, bffAuth{}, local)
	for _, token := range []string{"", "invalid", "readonly", "valid"} {
		req := httptest.NewRequest("POST", "/api/v1/calls/call/hangup", nil)
		req.Header.Set("X-Principal", `{"UserID":"forged","Role":"admin"}`)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		want := http.StatusUnauthorized
		if token == "valid" {
			want = http.StatusNoContent
		} else if token == "readonly" {
			want = http.StatusForbidden
		}
		if rec.Code != want {
			t.Fatalf("token=%q status=%d body=%s", token, rec.Code, rec.Body.String())
		}
	}
	if requests != 1 {
		t.Fatalf("unauthenticated request reached switch (%d)", requests)
	}
	for _, path := range []string{"/api/v1/calls/c/wrap-up", "/api/v1/calls/c/qa-marks", "/api/v1/calls/inbound"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", path, nil))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("business route intercepted: %s", path)
		}
	}
}
