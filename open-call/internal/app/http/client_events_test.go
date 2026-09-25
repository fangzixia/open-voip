package http

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"open-call/internal/app/http/middleware"
	"open-call/internal/layers/biz/auth"
	"open-call/internal/observability"
	"open-call/internal/ports"
)

type fakeCallLookup struct{ view ports.CallView }

func (f fakeCallLookup) GetCall(context.Context, string) (ports.CallView, error) { return f.view, nil }

func TestClientEventsGuestOwnershipAndSanitization(t *testing.T) {
	var trace bytes.Buffer
	observability.SetRecorder(observability.NewRecorder(&trace))
	t.Cleanup(func() { observability.SetRecorder(nil) })
	deps := RouterDeps{}

	body := `{"events":[{"type":"webrtc.state","call_id":"call-1","client_session_id":"browser-1","fields":{"token":"secret","state":"connected"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/client-events", strings.NewReader(body))
	req = req.WithContext(middleware.WithPrincipal(req.Context(), auth.Principal{
		Role: "guest", GuestID: "guest-1", GuestCallID: "call-1",
	}))
	rec := httptest.NewRecorder()
	deps.handleClientEvents(rec, req)
	if rec.Code != http.StatusAccepted || !strings.Contains(trace.String(), `"event":"client.webrtc.state"`) {
		t.Fatalf("status=%d body=%s trace=%s", rec.Code, rec.Body, trace.String())
	}
	if strings.Contains(trace.String(), "secret") {
		t.Fatalf("token leaked: %s", trace.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/client-events", strings.NewReader(
		`{"events":[{"type":"webrtc.state","call_id":"other-call"}]}`,
	))
	req = req.WithContext(middleware.WithPrincipal(req.Context(), auth.Principal{
		Role: "guest", GuestID: "guest-1", GuestCallID: "call-1",
	}))
	rec = httptest.NewRecorder()
	deps.handleClientEvents(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d: %s", rec.Code, rec.Body)
	}
}

func TestClientEventsAgentCallOwnership(t *testing.T) {
	deps := RouterDeps{Calls: fakeCallLookup{view: ports.CallView{ID: "call-1", AgentID: "agent-1"}}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/client-events", strings.NewReader(
		`{"events":[{"type":"media.ready","call_id":"call-1"}]}`,
	))
	req = req.WithContext(middleware.WithPrincipal(req.Context(), auth.Principal{
		Role: "agent", UserID: "user-1", AgentID: "agent-1",
	}))
	rec := httptest.NewRecorder()
	deps.handleClientEvents(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
}
