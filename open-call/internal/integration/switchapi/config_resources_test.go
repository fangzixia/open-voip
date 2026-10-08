package switchapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"open-call/internal/config"
	"open-call/internal/httpapi"
	"open-call/internal/ports"
	"testing"
)

func TestQueueUpdateTransmitsClearedBindings(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/switch/v1/queues/q/config" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for _, key := range []string{"ivr_flow_id", "post_call_ivr_flow_id", "overflow_queue_id"} {
			if v, ok := body[key]; !ok || v != "" {
				t.Errorf("clear omitted for %s: %+v", key, body)
			}
		}
		if body["audio_profile"] != "hd_webrtc" {
			t.Errorf("profile lost: %+v", body)
		}
		httpapi.Write(w, http.StatusOK, body)
	}))
	defer peer.Close()
	_, err := NewClient(config.IntegrationConfig{SwitchBaseURL: peer.URL}).UpdateQueueConfig(context.Background(), "q", ports.SwitchQueueConfig{ID: "q", AudioProfile: "hd_webrtc"})
	if err != nil {
		t.Fatal(err)
	}
}
