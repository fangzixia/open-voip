package control

import (
	"context"
	"errors"
	"testing"

	"open-switch/internal/ports"
)

type failedRecordingMedia struct {
	*fakeMedia
	meta ports.RecordingMeta
}

func (f *failedRecordingMedia) RecordingInfo(context.Context, string) (ports.RecordingMeta, error) {
	return f.meta, nil
}
func (f *failedRecordingMedia) StopRecording(context.Context, string) error {
	f.stops++
	return errors.New("disk full")
}

type failedRecordingStore struct {
	saved []ports.RecordingMeta
	err   error
}

func (f *failedRecordingStore) Save(_ context.Context, meta ports.RecordingMeta) error {
	f.saved = append(f.saved, meta)
	return f.err
}

func TestRecordingFailureKeepsCallActiveAndReportsPartialFiles(t *testing.T) {
	for _, storeErr := range []error{nil, errors.New("database unavailable")} {
		svc, base, _, events := newTestService("")
		media := &failedRecordingMedia{fakeMedia: base, meta: ports.RecordingMeta{ID: "rec1", CallID: "call1", FilePath: "partial.wav", LegPaths: map[string]string{"customer": "customer.wav", "agent": "agent.wav"}, RecordingSemantics: "conversation_mono_v1", Channels: 1, SampleRateHz: 8000, DurationSamples: 320, Status: "failed", FailureReason: "disk full"}}
		store := &failedRecordingStore{err: storeErr}
		svc.deps.Media, svc.deps.Recordings = media, store
		svc.calls["call1"] = &runtimeCall{rec: ports.CallRecord{ID: "call1", State: stateActive}, recordingID: "rec1"}
		svc.OnRecordingFailed(context.Background(), "rec1")
		if base.closed || base.stops != 0 || svc.calls["call1"].rec.State != stateActive {
			t.Fatal("recording failure altered active call")
		}
		if len(store.saved) != 1 || store.saved[0].Status != "failed" {
			t.Fatal("failed metadata was not attempted")
		}
		payload := events.payloads["recording.failed"]
		if len(payload) != 1 || payload[0]["recording_id"] != "rec1" || payload[0]["failure_reason"] != "disk full" || payload[0]["file_path"] != "partial.wav" || payload[0]["duration_samples"] != int64(320) {
			t.Fatal("incomplete failure event", payload)
		}
		if paths, ok := payload[0]["leg_paths"].(map[string]string); !ok || len(paths) != 2 {
			t.Fatal("partial stems missing", payload)
		}
		store.err = nil
		if err := svc.stopRecording(context.Background(), "call1"); err != nil {
			t.Fatal("recording stop failure prevents call cleanup", err)
		}
		if base.closed || svc.calls["call1"].rec.State != stateActive {
			t.Fatal("stop failure ended call")
		}
	}
}
