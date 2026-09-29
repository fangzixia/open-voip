package store

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"uuid"

	"open-call/internal/config"
	"open-call/internal/layers/biz/auth"
	"open-call/internal/layers/biz/authz"
	"open-call/internal/store/models"
)

// SeedIfEmpty 当 users 表为空时写入演示用管理员与坐席（呼叫配置由 SeedSwitchDemoConfig 写入 Switch）。
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
		EmployeeNo:   "ADMIN001",
		PasswordHash: adminHash,
		Role:         "admin",
		DisplayName:  "管理员",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	agentUser := models.User{
		ID:           uuid.New().String(),
		Username:     "agent1",
		EmployeeNo:   "AGENT001",
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
		EmployeeNo:   "AGENT002",
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
		EmployeeNo:   "SUPERVISOR001",
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
		return tx.Create(&supAg).Error
	})
	if err != nil {
		return fmt.Errorf("空库种子: %w", err)
	}
	if log != nil {
		log.Info("已写入演示种子账号", "admin", "admin", "agent1", "agent1", "agent2", "agent2", "supervisor", "supervisor")
	}
	return nil
}
