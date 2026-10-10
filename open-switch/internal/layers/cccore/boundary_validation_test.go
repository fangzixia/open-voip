package cccore

import (
	"github.com/google/uuid"
	"open-switch/internal/ports"
	"testing"
)

func TestQueueTechnicalConfigurationAuthority(t *testing.T) {
	for _, profile := range []string{"narrowband", "wideband", "hd_webrtc"} {
		q1, q2 := uuid.New().String(), uuid.New().String()
		bundle := ports.ConfigBundle{Queues: []ports.QueueConfig{
			{ID: q1, Name: "video", AudioProfile: profile, RecordingPolicy: "video_composite", AfterHoursAction: "queue", OverflowQueueID: q2},
			{ID: q2, Name: "overflow"},
		}}
		normalizeBundle(&bundle)
		if profile != "narrowband" {
			if err := validateBundle(bundle); err == nil {
				t.Fatalf("unsupported profile accepted: %s", profile)
			}
			continue
		}
		if err := validateBundle(bundle); err != nil {
			t.Fatalf("valid config %s: %v", profile, err)
		}
		bundle.Queues[0].AudioProfile = "unknown"
		if err := validateBundle(bundle); err == nil {
			t.Fatal("unknown profile accepted")
		}
		bundle.Queues[0].AudioProfile = profile
		bundle.Queues[0].RecordingPolicy = "unknown"
		if err := validateBundle(bundle); err == nil {
			t.Fatal("unknown recording policy accepted")
		}
		bundle.Queues[0].RecordingPolicy = "video_composite"
		bundle.Queues[0].MaxWaitSec = -1
		if err := validateBundle(bundle); err == nil {
			t.Fatal("negative wait accepted")
		}
	}
}

func TestGenericInputValidation(t *testing.T) {
	valid := `{"start":"input","nodes":{"input":{"type":"collect_input","accepted_digits":"09*#","result_key":"reference","next":"end","default":"end"},"end":{"type":"hangup"}}}`
	if err := validateIVR(valid, nil); err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{
		`{"start":"input","nodes":{"input":{"type":"csat"}}}`,
		`{"start":"missing","nodes":{"end":{"type":"hangup"}}}`,
		`{"start":"input","nodes":{"input":{"type":"collect_input","accepted_digits":"A","result_key":"reference","next":"end","default":"end"},"end":{"type":"hangup"}}}`,
		`{"start":"input","nodes":{"input":{"type":"collect_input","accepted_digits":"1","next":"end","default":"end"},"end":{"type":"hangup"}}}`,
		`{"start":"input","nodes":{"input":{"type":"collect_input","accepted_digits":"1","result_key":"reference","next":"end"},"end":{"type":"hangup"}}}`,
	} {
		if err := validateIVR(payload, nil); err == nil {
			t.Fatalf("invalid payload accepted: %s", payload)
		}
	}
}
