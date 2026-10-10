package cccore

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"open-switch/internal/datetime"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"

	"open-switch/internal/ports"
	"open-switch/internal/store/models"
)

// Upsert 写入 Switch 侧技术话单；业务系统通过事件流自行维护增强投影。
func (s *Service) Upsert(ctx context.Context, req ports.CDRWriteRequest) error {
	now := time.Now().UTC()
	duration, wait := 0, 0
	if req.EndedAt != nil && req.AnsweredAt != nil {
		duration = int(req.EndedAt.Sub(*req.AnsweredAt).Seconds())
	}
	if req.AnsweredAt != nil {
		wait = int(req.AnsweredAt.Sub(req.StartedAt).Seconds())
	} else if req.EndedAt != nil {
		wait = int(req.EndedAt.Sub(req.StartedAt).Seconds())
	}
	row := models.CDR{ID: uuid.New().String(), CallID: req.CallID, Direction: req.Direction, QueueID: uuidPointer(req.QueueID), AgentID: uuidPointer(req.AgentID), Caller: req.Caller, Callee: req.Callee, SessionType: string(req.SessionType), Result: req.Result, StartedAt: req.StartedAt, AnsweredAt: req.AnsweredAt, EndedAt: req.EndedAt, DurationSec: duration, WaitSec: wait, VideoStartedAt: req.VideoStartedAt, VideoUpgradeOk: req.VideoUpgradeOk, ScreenShareCount: req.ScreenShareCount, CreatedAt: now}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "call_id"}}, DoUpdates: clause.AssignmentColumns([]string{"direction", "queue_id", "agent_id", "caller", "callee", "session_type", "result", "started_at", "answered_at", "ended_at", "duration_sec", "wait_sec", "video_started_at", "video_upgrade_ok", "screen_share_count"})}).Create(&row).Error; err != nil {
			return err
		}
		raw, err := datetime.Marshal(req)
		if err != nil {
			return err
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			return err
		}
		return s.events.WithDB(tx).PublishCallEvent(ctx, ports.CallEvent{CallID: req.CallID, Type: "cdr.updated", Payload: payload})
	})
}

// Save 在 Switch 库中保存录音技术元数据并发布 recording.saved 事件。
func (s *Service) Save(ctx context.Context, rec ports.RecordingMeta) error {
	if rec.LegPaths == nil {
		rec.LegPaths = map[string]string{}
	}
	row := models.Recording{ID: rec.ID, CallID: rec.CallID, FilePath: rec.FilePath, MediaType: rec.MediaType, StartedAt: rec.StartedAt, EndedAt: rec.EndedAt, RetainUntil: rec.RetainUntil, FileSize: rec.FileSize, CreatedAt: time.Now().UTC()}
	row.RecordingSemantics, row.Channels, row.DurationSamples, row.Status, row.FailureReason = rec.RecordingSemantics, rec.Channels, rec.DurationSamples, rec.Status, rec.FailureReason
	row.SampleRateHz, row.LegPaths = rec.SampleRateHz, rec.LegPaths
	updates := clause.AssignmentColumns([]string{"file_path", "media_type", "started_at", "retain_until", "recording_semantics", "channels", "status", "sample_rate_hz"})
	for name, expression := range map[string]string{
		"duration_samples": "GREATEST(os_recordings.duration_samples, EXCLUDED.duration_samples)",
		"file_size":        "GREATEST(os_recordings.file_size, EXCLUDED.file_size)",
		"ended_at":         "GREATEST(os_recordings.ended_at, EXCLUDED.ended_at)",
		"leg_paths":        "os_recordings.leg_paths || EXCLUDED.leg_paths",
		"failure_reason":   "COALESCE(NULLIF(os_recordings.failure_reason, ''), EXCLUDED.failure_reason)",
	} {
		updates = append(updates, clause.Assignment{Column: clause.Column{Name: name}, Value: gorm.Expr(expression)})
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, Where: clause.Where{Exprs: []clause.Expression{gorm.Expr("(os_recordings.status <> 'failed' OR EXCLUDED.status = 'failed') AND (EXCLUDED.status <> 'recording' OR os_recordings.status = 'recording')")}}, DoUpdates: updates}).Create(&row).Error; err != nil {
			return err
		}
		// Publish the committed merge, rather than an older incoming snapshot.
		if err := tx.First(&row, "id = ?", rec.ID).Error; err != nil {
			return err
		}
		rec = ports.RecordingMeta{
			ID: row.ID, CallID: row.CallID, FilePath: row.FilePath, MediaType: row.MediaType,
			StartedAt: row.StartedAt, EndedAt: row.EndedAt, RetainUntil: row.RetainUntil,
			FileSize: row.FileSize, RecordingSemantics: row.RecordingSemantics, Channels: row.Channels,
			DurationSamples: row.DurationSamples, Status: row.Status, FailureReason: row.FailureReason,
			SampleRateHz: row.SampleRateHz, LegPaths: row.LegPaths,
		}
		raw, err := datetime.Marshal(rec)
		if err != nil {
			return err
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			return err
		}
		return s.events.WithDB(tx).PublishCallEvent(ctx, ports.CallEvent{CallID: rec.CallID, Type: "recording.saved", Payload: payload})
	})
}
