package models

import "time"

// ConfigVersion 已校验、不可变的路由配置快照。
type ConfigVersion struct {
	Version     int64     `gorm:"primaryKey"`
	Status      string    `gorm:"not null"`
	Checksum    string    `gorm:"uniqueIndex;not null"`
	Payload     string    `gorm:"type:text;not null"`
	CreatedAt   time.Time `gorm:"not null"`
	ActivatedAt *time.Time
}

func (ConfigVersion) TableName() string { return "os_config_versions" }

// ActiveConfig 指向新通话所使用的已激活配置快照（单行，id 固定为 1）。
type ActiveConfig struct {
	ID          int16     `gorm:"primaryKey"`
	Version     int64     `gorm:"not null"`
	ActivatedAt time.Time `gorm:"not null"`
}

func (ActiveConfig) TableName() string { return "os_active_config" }
