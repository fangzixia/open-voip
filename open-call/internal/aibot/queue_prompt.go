package aibot

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"open-call/internal/config"
	"open-call/internal/store/models"
)

// QueuePromptStore 读取队列级 system_prompt（管理端 oc_aibot_queue_profiles）。
type QueuePromptStore struct {
	DB *gorm.DB
}

func (s QueuePromptStore) ForQueue(ctx context.Context, queueID string) string {
	if s.DB == nil || queueID == "" {
		return ""
	}
	var row models.AibotQueueProfile
	if err := s.DB.WithContext(ctx).Where("queue_id = ?", queueID).First(&row).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(row.SystemPrompt)
}

func resolveSystemPrompt(ctx context.Context, cfg config.AibotConfig, prompts QueuePromptStore, queueID string) string {
	if p := prompts.ForQueue(ctx, queueID); p != "" {
		return p
	}
	return cfg.SystemPrompt
}
