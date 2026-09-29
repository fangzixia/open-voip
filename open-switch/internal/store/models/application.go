package models

import "time"

// Application 登记的业务系统集成方。
type Application struct {
	ID                 string `gorm:"primaryKey;size:64"`
	SecretHash         string `gorm:"column:secret_hash;size:128;not null"`
	Secret             string `gorm:"column:secret;size:128;not null"`
	EventsCallbackURL  string `gorm:"column:events_callback_url;size:512;not null"`
	EventRetentionDays int    `gorm:"column:event_retention_days;not null"`
	MaxConcurrentCalls int    `gorm:"column:max_concurrent_calls;not null"`
	Enabled            bool   `gorm:"not null;default:true"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (Application) TableName() string { return "os_applications" }
