package store

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-call/internal/config"
	"open-call/internal/layers/biz/auth"
	"open-call/internal/layers/biz/authz"
	"open-call/internal/store/models"
)

// SeedIfEmpty 当 users 表为空时写入 Demo 管理员、坐席与队列。
func SeedIfEmpty(db *gorm.DB, cfg config.BootstrapConfig, log *slog.Logger) error {
	if !cfg.Enabled {
		return nil
	}
	var n int64
	if err := db.Model(&models.User{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	adminHash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	agentHash, err := auth.HashPassword(cfg.AgentPassword)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	admin := models.User{
		ID:           uuid.New().String(),
		Username:     "admin",
		PasswordHash: adminHash,
		Role:         "admin",
		DisplayName:  "管理员",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	agentUser := models.User{
		ID:           uuid.New().String(),
		Username:     "agent1",
		PasswordHash: agentHash,
		Role:         "agent",
		DisplayName:  "坐席一",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	ag := models.Agent{
		ID:           uuid.New().String(),
		UserID:       agentUser.ID,
		Extension:    "1001",
		VideoCapable: true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	agentUser2 := models.User{
		ID:           uuid.New().String(),
		Username:     "agent2",
		PasswordHash: agentHash,
		Role:         "agent",
		DisplayName:  "坐席二",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	ag2 := models.Agent{
		ID:           uuid.New().String(),
		UserID:       agentUser2.ID,
		Extension:    "1002",
		VideoCapable: true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	supUser := models.User{
		ID:           uuid.New().String(),
		Username:     "supervisor",
		PasswordHash: agentHash,
		Role:         "supervisor",
		DisplayName:  "班长",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	supAg := models.Agent{
		ID:           uuid.New().String(),
		UserID:       supUser.ID,
		Extension:    "1099",
		VideoCapable: true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	audioQ := models.Queue{
		ID:                uuid.New().String(),
		Name:              "语音服务",
		VideoEnabled:      false,
		MaxWaitSec:        300,
		DispatchStrategy:  "longest_idle",
		RecordingPolicy:   "audio",
		AnnounceRecording: true,
		OverflowAction:    "hangup",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	videoQ := models.Queue{
		ID:                uuid.New().String(),
		Name:              "视频服务",
		VideoEnabled:      true,
		MaxWaitSec:        300,
		DispatchStrategy:  "longest_idle",
		RecordingPolicy:   "video_composite",
		AnnounceRecording: true,
		OverflowAction:    "queue",
		OverflowQueueID:   &audioQ.ID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&admin).Error; err != nil {
			return err
		}
		if err := tx.Create(&agentUser).Error; err != nil {
			return err
		}
		if err := tx.Create(&ag).Error; err != nil {
			return err
		}
		if err := tx.Create(&agentUser2).Error; err != nil {
			return err
		}
		if err := tx.Create(&ag2).Error; err != nil {
			return err
		}
		if err := tx.Create(&supUser).Error; err != nil {
			return err
		}
		for _, u := range []models.User{admin, agentUser, agentUser2, supUser} {
			if err := tx.Create(&authz.UserRole{UserID: u.ID, RoleID: u.Role}).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&supAg).Error; err != nil {
			return err
		}
		if err := tx.Create(&audioQ).Error; err != nil {
			return err
		}
		if err := tx.Create(&videoQ).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.QueueAgent{QueueID: audioQ.ID, AgentID: ag.ID}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.QueueAgent{QueueID: videoQ.ID, AgentID: ag.ID}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.QueueAgent{QueueID: audioQ.ID, AgentID: ag2.ID}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.QueueAgent{QueueID: videoQ.ID, AgentID: ag2.ID}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.QueueAgent{QueueID: audioQ.ID, AgentID: supAg.ID}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.QueueAgent{QueueID: videoQ.ID, AgentID: supAg.ID}).Error; err != nil {
			return err
		}
		return tx.Create(&models.DIDRoute{
			ID: uuid.New().String(), DID: "8001", QueueID: audioQ.ID, DisplayName: "语音服务", CreatedAt: now, UpdatedAt: now,
		}).Error
	})
	if err != nil {
		return fmt.Errorf("空库种子: %w", err)
	}
	if log != nil {
		log.Info("已写入 Demo 种子账号", "admin", "admin", "agent", "agent1", "agent2", "agent2", "supervisor", "supervisor", "did", "8001")
	}
	return nil
}

// SeedDefaultDID 当 did_routes 为空时写入 8001 → 语音服务，供 MicroSIP 呼入演示。
func SeedDefaultDID(db *gorm.DB, log *slog.Logger) error {
	var n int64
	if err := db.Model(&models.DIDRoute{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	var q models.Queue
	if err := db.Where("name = ?", "语音服务").First(&q).Error; err != nil {
		return nil
	}
	now := time.Now().UTC()
	row := models.DIDRoute{
		ID: uuid.New().String(), DID: "8001", QueueID: q.ID, DisplayName: "语音服务", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	if log != nil {
		log.Info("已写入默认 DID", "did", "8001", "queue", q.Name)
	}
	return nil
}
