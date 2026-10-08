package queue

import (
	"context"
	"errors"
	"open-call/internal/errs"
	"open-call/internal/ports"
	"testing"
)

type queueSwitch struct {
	ports.SwitchAdminPort
	current  ports.SwitchQueueConfig
	received ports.SwitchQueueConfig
	err      error
}

func (s *queueSwitch) GetQueueConfig(context.Context, string) (ports.SwitchQueueConfig, error) {
	return s.current, nil
}
func (s *queueSwitch) CreateQueueConfig(_ context.Context, in ports.SwitchQueueConfig) (ports.SwitchQueueConfig, error) {
	s.received = in
	return in, s.err
}
func (s *queueSwitch) UpdateQueueConfig(_ context.Context, _ string, in ports.SwitchQueueConfig) (ports.SwitchQueueConfig, error) {
	s.received = in
	return in, s.err
}

func TestQueueTechnicalCapabilitiesAreValidatedBySwitch(t *testing.T) {
	sw := &queueSwitch{}
	svc := NewService(sw, nil)
	_, err := svc.Create(context.Background(), CreateInput{Name: "Video", AudioProfile: "wideband", RecordingPolicy: "video_composite", AfterHoursAction: "queue", OverflowQueueID: "overflow"})
	if err != nil {
		t.Fatal(err)
	}
	if sw.received.AudioProfile != "wideband" || sw.received.RecordingPolicy != "video_composite" || sw.received.AfterHoursAction != "queue" {
		t.Fatalf("capabilities changed: %+v", sw.received)
	}
	sw.err = errs.InvalidRequest("switch rejected capability")
	_, err = svc.Create(context.Background(), CreateInput{Name: "Future", AudioProfile: "future_profile", MaxWaitSec: -1})
	if !errors.Is(err, sw.err) || sw.received.AudioProfile != "future_profile" || sw.received.MaxWaitSec != -1 {
		t.Fatalf("call masked authoritative validation: %+v %v", sw.received, err)
	}
}

func TestQueuePatchClearsBindingsAndPreservesOmittedFields(t *testing.T) {
	sw := &queueSwitch{current: ports.SwitchQueueConfig{ID: "q", Name: "Support", VideoEnabled: true, AudioProfile: "wideband", MaxWaitSec: 200, RecordingPolicy: "video_composite", IVRFlowID: "entry", PostCallIVRFlowID: "followup", OverflowQueueID: "overflow"}}
	empty := ""
	_, err := NewService(sw, nil).Update(context.Background(), "q", UpdateInput{IVRFlowID: &empty, PostCallIVRFlowID: &empty, OverflowQueueID: &empty})
	if err != nil {
		t.Fatal(err)
	}
	got := sw.received
	if got.IVRFlowID != "" || got.PostCallIVRFlowID != "" || got.OverflowQueueID != "" || !got.VideoEnabled || got.AudioProfile != "wideband" || got.MaxWaitSec != 200 || got.RecordingPolicy != "video_composite" {
		t.Fatalf("patch lost values: %+v", got)
	}
}
