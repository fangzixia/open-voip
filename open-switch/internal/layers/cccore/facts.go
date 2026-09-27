package cccore

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"

	"open-switch/internal/ports"
	"open-switch/internal/scope"
	"open-switch/internal/store/models"
)

// Upsert stores the Switch-owned technical CDR. Business systems enrich their own projection from events.
func (s *Service) Upsert(ctx context.Context, req ports.CDRWriteRequest) error {
	appID := scope.Application(ctx)
	if appID == "" {
		_ = s.db.WithContext(ctx).Raw("SELECT application_id FROM os_calls WHERE id = ?", req.CallID).Scan(&appID).Error
	}
	now := time.Now().UTC()
	duration, wait := 0, 0
	if req.EndedAt != nil {
		duration = int(req.EndedAt.Sub(req.StartedAt).Seconds())
	}
	if req.AnsweredAt != nil {
		wait = int(req.AnsweredAt.Sub(req.StartedAt).Seconds())
	}
	row := models.CDR{ID: uuid.NewString(), ApplicationID: appID, CallID: req.CallID, Direction: req.Direction, QueueID: uuidPointer(req.QueueID), AgentID: uuidPointer(req.AgentID), Caller: req.Caller, Callee: req.Callee, SessionType: string(req.SessionType), Result: req.Result, StartedAt: req.StartedAt, AnsweredAt: req.AnsweredAt, EndedAt: req.EndedAt, DurationSec: duration, WaitSec: wait, VideoStartedAt: req.VideoStartedAt, VideoUpgradeOk: req.VideoUpgradeOk, ScreenShareCount: req.ScreenShareCount, CreatedAt: now}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "call_id"}}, DoUpdates: clause.AssignmentColumns([]string{"application_id", "direction", "queue_id", "agent_id", "caller", "callee", "session_type", "result", "started_at", "answered_at", "ended_at", "duration_sec", "wait_sec", "video_started_at", "video_upgrade_ok", "screen_share_count"})}).Create(&row).Error
}

// Save stores recording technical metadata in the Switch database.
func (s *Service) Save(ctx context.Context, rec ports.RecordingMeta) error {
	appID := scope.Application(ctx)
	if appID == "" {
		_ = s.db.WithContext(ctx).Raw("SELECT application_id FROM os_calls WHERE id = ?", rec.CallID).Scan(&appID).Error
	}
	row := models.Recording{ID: rec.ID, ApplicationID: appID, CallID: rec.CallID, FilePath: rec.FilePath, MediaType: rec.MediaType, StartedAt: rec.StartedAt, EndedAt: rec.EndedAt, RetainUntil: rec.RetainUntil, FileSize: rec.FileSize, CreatedAt: time.Now().UTC()}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"file_path", "media_type", "started_at", "ended_at", "retain_until", "file_size"})}).Create(&row).Error
}
