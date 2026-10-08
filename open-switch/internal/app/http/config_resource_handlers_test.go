package http

import (
	"context"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"open-switch/internal/ports"
	"strings"
	"testing"
)

type queueAdmin struct {
	ports.CallCenterAdminPort
	received ports.QueueConfig
}

func (s *queueAdmin) GetQueueConfig(context.Context, string) (ports.QueueConfig, error) {
	return ports.QueueConfig{ID: "q", Name: "Support", AudioProfile: "narrowband", IVRFlowID: "entry", PostCallIVRFlowID: "followup", OverflowQueueID: "overflow"}, nil
}
func (s *queueAdmin) UpdateQueueConfig(_ context.Context, _ string, in ports.QueueConfig) (ports.QueueConfig, error) {
	s.received = in
	return in, nil
}

func TestQueuePatchPreservesOmissionAndHonorsExplicitValues(t *testing.T) {
	for _, tc := range []struct {
		body, profile, entry, followup, overflow string
		wait                                     int
	}{
		{`{"name":"renamed"}`, "narrowband", "entry", "followup", "overflow", 0},
		{`{"audio_profile":"wideband","ivr_flow_id":"","post_call_ivr_flow_id":"","overflow_queue_id":"","max_wait_sec":-1}`, "wideband", "", "", "", -1},
		{`{"post_call_ivr_flow_id":"new-flow"}`, "narrowband", "entry", "new-flow", "overflow", 0},
	} {
		t.Run(tc.body, func(t *testing.T) {
			admin := &queueAdmin{}
			deps := SwitchRouterDeps{Admin: admin}
			router := chi.NewRouter()
			router.Patch("/queues/{queueId}/config", deps.handleQueueConfigPatch)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/queues/q/config", strings.NewReader(tc.body)))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
			}
			got := admin.received
			if got.AudioProfile != tc.profile || got.IVRFlowID != tc.entry || got.PostCallIVRFlowID != tc.followup || got.OverflowQueueID != tc.overflow || got.MaxWaitSec != tc.wait {
				t.Fatalf("incorrect merge: %+v", got)
			}
		})
	}
}
