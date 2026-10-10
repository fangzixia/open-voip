package control

import (
	"context"
	"open-switch/internal/observability"
	"time"
)

// Called asynchronously by media; recording errors never acquire or stop the
// room's audio clock. The event includes partial files and recording semantics.
func (s *Service) OnRecordingFailed(ctx context.Context, id string) {
	meta, e := s.deps.Media.RecordingInfo(ctx, id)
	if e != nil {
		observability.Event(ctx, "recording", "recording.failure_report", "metadata", "error", "recording_info_failed", time.Time{}, "recording_id", id)
		return
	}
	if s.deps.Recordings != nil {
		if err := s.deps.Recordings.Save(ctx, meta); err != nil {
			observability.Event(ctx, "recording", "recording.failure_report", "persist", "error", "recording_metadata_save_failed", time.Time{}, "recording_id", id, "call_id", meta.CallID)
		}
	}
	if err := s.publishCall(ctx, meta.CallID, "recording.failed", "", map[string]any{
		"id": meta.ID, "recording_id": meta.ID, "call_id": meta.CallID, "file_path": meta.FilePath,
		"media_type": meta.MediaType, "started_at": meta.StartedAt, "ended_at": meta.EndedAt,
		"file_size": meta.FileSize, "leg_paths": meta.LegPaths, "sample_rate_hz": meta.SampleRateHz,
		"recording_semantics": meta.RecordingSemantics, "channels": meta.Channels, "duration_samples": meta.DurationSamples,
		"status": "failed", "failure_reason": meta.FailureReason,
		"message": "录音保存失败，通话将继续。已录制的部分可在录音页面查看。",
	}); err != nil {
		observability.Event(ctx, "recording", "recording.failure_report", "publish", "error", "recording_failure_event_failed", time.Time{}, "recording_id", id, "call_id", meta.CallID)
	}
}
