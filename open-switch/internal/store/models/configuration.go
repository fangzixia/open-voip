package models

import "time"

// ConfigVersion 已校验、不可变的应用路由配置快照。
type ConfigVersion struct {
	ApplicationID string    `gorm:"primaryKey;size:64"`
	Version       int64     `gorm:"primaryKey"`
	Status        string    `gorm:"not null"`
	Checksum      string    `gorm:"not null"`
	Payload       string    `gorm:"type:text;not null"`
	CreatedAt     time.Time `gorm:"not null"`
	ActivatedAt   *time.Time
}

func (ConfigVersion) TableName() string { return "os_config_versions" }

// ActiveConfig 指向新通话所使用的已激活配置快照。
type ActiveConfig struct {
	ApplicationID string    `gorm:"primaryKey;size:64"`
	Version       int64     `gorm:"not null"`
	ActivatedAt   time.Time `gorm:"not null"`
}

func (ActiveConfig) TableName() string { return "os_active_config" }
