package aibot

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-call/internal/ports"
	"open-call/internal/store/models"
)

// UsageRecorder 记录 AI 会话消费并派发 webhook。
type UsageRecorder struct {
	db    *gorm.DB
	hooks ports.WebhookDispatcher
	log   *slog.Logger
}

func NewUsageRecorder(db *gorm.DB, hooks ports.WebhookDispatcher, log *slog.Logger) *UsageRecorder {
	return &UsageRecorder{db: db, hooks: hooks, log: log}
}

func (u *UsageRecorder) RecordAISession(callID, agentID, queueID string, started time.Time, inputMs, outputMs int64) {
	if u == nil || u.db == nil {
		return
	}
	row := models.AibotUsage{
		ID:            uuid.New().String(),
		CallID:        callID,
		AgentID:       agentID,
		QueueID:       queueID,
		InputAudioMs:  inputMs,
		OutputAudioMs: outputMs,
		StartedAt:     started.UTC(),
		EndedAt:       time.Now().UTC(),
	}
	if err := u.db.Create(&row).Error; err != nil && u.log != nil {
		u.log.Warn("AI 用量落库失败", "call_id", callID, "err", err)
	}
	if u.hooks != nil {
		_ = u.hooks.Dispatch(context.Background(), "ai.session.completed", map[string]any{
			"call_id":          callID,
			"agent_id":         agentID,
			"queue_id":         queueID,
			"input_audio_ms":   inputMs,
			"output_audio_ms":  outputMs,
			"duration_ms":      time.Since(started).Milliseconds(),
		})
	}
}
