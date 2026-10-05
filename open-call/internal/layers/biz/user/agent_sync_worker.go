package user

import (
	"context"
	"log/slog"
	"time"

	"uuid"

	"gorm.io/gorm"

	"open-call/internal/store/models"
)

func enqueueAgentSync(tx *gorm.DB, agentID string) error {
	_ = tx.Where("agent_id = ?", agentID).Delete(&models.AgentSyncTask{}).Error
	row := models.AgentSyncTask{
		ID: uuid.New().String(), AgentID: agentID,
		NextRetryAt: time.Now().UTC(), CreatedAt: time.Now().UTC(),
	}
	return tx.Create(&row).Error
}

// RunAgentSyncWorker 重试失败的坐席同步任务。
func RunAgentSyncWorker(ctx context.Context, db *gorm.DB, svc *Service, log *slog.Logger) {
	if svc == nil || svc.sw == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		svc.flushAgentSync(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) flushAgentSync(ctx context.Context) {
	var tasks []models.AgentSyncTask
	if err := s.db.WithContext(ctx).Where("next_retry_at <= NOW()").Order("next_retry_at").Limit(20).Find(&tasks).Error; err != nil {
		return
	}
	for _, task := range tasks {
		var ag models.Agent
		if err := s.db.WithContext(ctx).First(&ag, "id = ?", task.AgentID).Error; err != nil {
			_ = s.db.Delete(&models.AgentSyncTask{}, "id = ?", task.ID).Error
			continue
		}
		err := s.syncAgent(ctx, ag)
		if err == nil {
			_ = s.db.Delete(&models.AgentSyncTask{}, "id = ?", task.ID).Error
			continue
		}
		attempts := task.Attempts + 1
		next := time.Now().UTC().Add(time.Duration(min(attempts, 6)) * time.Second)
		status := map[string]any{"attempts": attempts, "next_retry_at": next, "last_error": err.Error()}
		if attempts >= 12 {
			_ = s.db.Delete(&models.AgentSyncTask{}, "id = ?", task.ID).Error
		} else {
			_ = s.db.Model(&models.AgentSyncTask{}).Where("id = ?", task.ID).Updates(status).Error
		}
	}
}
