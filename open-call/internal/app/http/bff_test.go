// 本文件验证bff的关键行为。
package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"open-call/internal/config"
	"open-call/internal/datetime"
	"open-call/internal/errs"
	"open-call/internal/layers/biz/auth"
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

func TestBFFAuthenticatesAndReplacesForgedIdentity(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer internal-secret" {
			t.Error("missing integration secret")
		}
		var p auth.Principal
		if err := datetime.Unmarshal([]byte(r.Header.Get("X-Principal")), &p); err != nil {
			t.Error(err)
		}
		if p.UserID != "user" || p.Role != "agent" || p.AgentID != "seat" {
			t.Errorf("forged principal survived: %+v", p)
		}
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
