// Package store 提供 GORM 与 PostgreSQL 连接及 AutoMigrate，持久化模型主要供 L4 使用。
package store

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"open-voip/internal/store/models"
)

// Open 打开 PostgreSQL 连接并配置 GORM。
func Open(dsn string, logLevel logger.LogLevel) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("连接数据库: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// AutoMigrate 按依赖顺序迁移全部模型表结构。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.Skill{},
		&models.Agent{},
		&models.AgentSkill{},
		&models.Queue{},
		&models.QueueAgent{},
		&models.QueueSkill{},
		&models.DIDRoute{},
		&models.AgentSession{},
		&models.AgentSessionQueue{},
		&models.AgentStateLog{},
		&models.Call{},
		&models.CallLeg{},
		&models.CDR{},
		&models.GuestSession{},
		&models.IVRFlow{},
		&models.IVRPublishedSnapshot{},
		&models.Recording{},
		&models.QAMark{},
		&models.CallWrapUp{},
		&models.WebhookSubscription{},
		&models.WebhookDelivery{},
		&models.AuditLog{},
		&models.JWTRevocation{},
	)
}

// Ping 检查数据库是否可用。
func Ping(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
