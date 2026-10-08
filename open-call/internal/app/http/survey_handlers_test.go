package http

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"open-call/internal/app/http/middleware"
	"open-call/internal/config"
	"open-call/internal/httpapi"
	"open-call/internal/integration/switchapi"
	"strings"
	"testing"
)

func TestSurveyBusinessEndpointUsesGenericIVRAndPinnedDefault(t *testing.T) {
	entered := 0
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/switch/v1/internal/calls/owned":
			httpapi.Write(w, 200, map[string]any{"id": "owned", "agent_id": "seat", "version": 9, "post_call_ivr_flow_id": "pinned-flow"})
		case "/switch/v1/internal/calls/foreign":
			httpapi.Write(w, 200, map[string]any{"id": "foreign", "agent_id": "other", "version": 9})
		case "/switch/v1/calls/owned/ivr":
			entered++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["flow_id"] != "pinned-flow" || body["expected_version"] != float64(9) || r.Header.Get("Idempotency-Key") != "survey-command" {
				t.Errorf("bad generic command: %+v", body)
			}
			httpapi.Write(w, 200, map[string]any{"id": "owned", "state": "ivr"})
		default:
			t.Errorf("business endpoint used unexpected Switch API: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer peer.Close()
	cfg := config.IntegrationConfig{SwitchBaseURL: peer.URL}
	deps := RouterDeps{Switch: switchapi.NewClient(cfg)}
	router := chi.NewRouter()
	router.Use(middleware.Auth(bffAuth{}))
	router.Use(middleware.RequirePermission("calls.operate"))
	router.Post("/api/v1/calls/{callId}/survey", deps.handleCallSurvey)
	handler := WrapSwitchBFF(cfg, bffAuth{}, router, nil, nil)
	for _, tc := range []struct {
		id, token string
		want      int
	}{{"owned", "valid", 200}, {"foreign", "valid", 403}, {"owned", "readonly", 403}, {"owned", "", 401}} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/calls/"+tc.id+"/survey", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("Idempotency-Key", "survey-command")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s/%s status=%d body=%s", tc.id, tc.token, rec.Code, rec.Body.String())
		}
	}
	if entered != 1 {
		t.Fatalf("generic IVR entered %d times", entered)
	}
}
