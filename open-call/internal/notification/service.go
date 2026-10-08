package notification

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"open-call/internal/errs"
	"open-call/internal/integration/switchapi"
	"open-call/internal/ports"
	"open-call/internal/ports/dto"
	"open-call/internal/store/models"
)

type API interface {
	CreateApplicationCall(context.Context, string, string) error
	DialApplicationSIP(context.Context, string, string, string) (string, error)
	GetCall(context.Context, string) (ports.CallView, error)
	PlayAsset(context.Context, string, string, string, string) (string, error)
	Playback(context.Context, string, string, string) (switchapi.PlaybackStatus, error)
	Hangup(context.Context, string, dto.HangupReason) error
}

type Service struct {
	db  *gorm.DB
	api API
	log *slog.Logger
}

func NewService(db *gorm.DB, api API, log *slog.Logger) *Service {
	return &Service{db: db, api: api, log: log}
}

func (s *Service) Submit(ctx context.Context, initiator, key, destination, trunk, asset string) (models.VoiceNotification, error) {
	destination = strings.TrimSpace(destination)
	asset = strings.TrimSpace(asset)
	if initiator == "" || destination == "" || asset == "" {
		return models.VoiceNotification{}, errs.InvalidRequest("发起人、号码和素材必填")
	}
	if _, err := uuid.Parse(strings.TrimSuffix(asset, ".wav")); err != nil || !strings.HasSuffix(asset, ".wav") {
		return models.VoiceNotification{}, errs.InvalidRequest("素材 ID 无效")
	}
	if key == "" {
		key = uuid.New().String()
	}
	if len(key) > 200 {
		return models.VoiceNotification{}, errs.InvalidRequest("幂等键过长")
	}
	now := time.Now().UTC()
	t := models.VoiceNotification{ID: uuid.New().String(), InitiatorID: initiator, RequestKey: key, Destination: destination, TrunkID: trunk, AssetID: asset, State: "queued", Deadline: now.Add(10 * time.Minute), CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&t).Error; err != nil {
		return t, err
	}
	var stored models.VoiceNotification
	err := s.db.WithContext(ctx).Where("initiator_id = ? AND request_key = ?", initiator, key).First(&stored).Error
	if err == nil && (stored.Destination != destination || stored.AssetID != asset || stored.TrunkID != trunk) {
		err = errs.Conflict("幂等键已用于另一项通知", "")
	}
	return stored, err
}
func (s *Service) Get(ctx context.Context, initiator, id string) (models.VoiceNotification, error) {
	var t models.VoiceNotification
	err := s.db.WithContext(ctx).Where("id = ? AND initiator_id = ?", id, initiator).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = errs.NotFound("通知任务不存在")
	}
	return t, err
}
func (s *Service) Cancel(ctx context.Context, initiator, id string) error {
	_, err := s.Get(ctx, initiator, id)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&models.VoiceNotification{}).Where("id = ? AND state NOT IN ?", id, []string{"completed", "failed", "canceled"}).Updates(map[string]any{"state": "stopping", "last_error": "canceled", "updated_at": time.Now().UTC()}).Error
}

// Run uses short DB leases so a Call restart resumes accepted business tasks.
// No AI configuration, seat check-in, media connection or local audio loop.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}
func (s *Service) tick(ctx context.Context) {
	var tasks []models.VoiceNotification
	if err := s.db.WithContext(ctx).Where("state NOT IN ? AND (locked_until IS NULL OR locked_until < ?)", []string{"completed", "failed", "canceled"}, time.Now().UTC()).Order("created_at").Limit(32).Find(&tasks).Error; err != nil {
		if s.log != nil {
			s.log.Warn("通知任务读取失败", "err", err)
		}
		return
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		lease := time.Now().UTC().Add(90 * time.Second)
		r := s.db.WithContext(ctx).Model(&models.VoiceNotification{}).Where("id = ? AND state NOT IN ? AND (locked_until IS NULL OR locked_until < ?)", task.ID, []string{"completed", "failed", "canceled"}, time.Now().UTC()).Update("locked_until", lease)
		if r.Error != nil || r.RowsAffected != 1 {
			continue
		}
		// Reload after acquiring the lease, including concurrent user cancellation.
		if s.db.WithContext(ctx).First(&task, "id = ?", task.ID).Error != nil {
			continue
		}
		old := task.State
		err := s.step(ctx, &task)
		if err != nil && ctx.Err() == nil {
			task.State = "stopping"
			if task.LastError == "" {
				task.LastError = err.Error()
			}
		}
		values := map[string]any{"state": task.State, "leg_id": task.LegID, "playback_id": task.PlaybackID, "last_error": task.LastError, "locked_until": nil, "updated_at": time.Now().UTC()}
		// Cancellation wins over an in-flight step. The next tick performs cleanup.
		update := s.db.WithContext(context.WithoutCancel(ctx)).Model(&models.VoiceNotification{}).Where("id = ? AND state = ? AND locked_until = ?", task.ID, old, lease).Updates(values)
		if update.RowsAffected == 0 {
			s.db.WithContext(context.WithoutCancel(ctx)).Model(&models.VoiceNotification{}).Where("id = ? AND locked_until = ?", task.ID, lease).Update("locked_until", nil)
		}
	}
}
func (s *Service) step(ctx context.Context, t *models.VoiceNotification) error {
	if t.State == "completed" || t.State == "failed" || t.State == "canceled" {
		return nil
	}
	if time.Now().After(t.Deadline) && t.State != "stopping" {
		t.State = "stopping"
		t.LastError = "notification deadline exceeded"
	}
	if t.State == "stopping" {
		err := s.api.Hangup(ctx, t.ID, dto.HangupReasonError)
		var api *errs.APIError
		if err != nil && !(errors.As(err, &api) && api.HTTP == 404) {
			return err
		}
		t.State = "failed"
		if t.LastError == "canceled" {
			t.State = "canceled"
		}
		return nil
	}
	if t.State == "queued" {
		if err := s.api.CreateApplicationCall(ctx, t.ID, t.ID); err != nil {
			return err
		}
		leg, err := s.api.DialApplicationSIP(ctx, t.ID, t.Destination, t.TrunkID)
		if err != nil {
			return err
		}
		t.LegID = leg
		t.State = "dialing"
		return nil
	}
	if t.State == "playing" {
		p, err := s.api.Playback(ctx, t.ID, t.LegID, t.PlaybackID)
		if err != nil {
			return err
		}
		switch p.State {
		case "finished":
			t.State = "finishing"
		case "stopped", "failed":
			return errors.New("notification playback interrupted")
		}
	}
	if t.State == "finishing" {
		if err := s.api.Hangup(ctx, t.ID, dto.HangupReasonNormal); err != nil {
			return err
		}
		t.State = "completed"
		return nil
	}
	view, err := s.api.GetCall(ctx, t.ID)
	if err != nil {
		return err
	}
	if view.State == "ended" {
		t.State = "failed"
		t.LastError = "call ended before notification completed"
		return nil
	}
	if t.State == "dialing" {
		if view.State != "active" {
			return nil
		}
		id, err := s.api.PlayAsset(ctx, t.ID, t.LegID, t.AssetID, "notification:"+t.ID)
		if err != nil {
			return err
		}
		t.PlaybackID = id
		t.State = "playing"
		return nil
	}

	return nil
}
