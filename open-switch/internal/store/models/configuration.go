package models

import "time"

// ConfigVersion is an immutable, validated application routing snapshot.
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

// ActiveConfig points to the snapshot used for new calls.
type ActiveConfig struct {
	ApplicationID string    `gorm:"primaryKey;size:64"`
	Version       int64     `gorm:"not null"`
	ActivatedAt   time.Time `gorm:"not null"`
}

func (ActiveConfig) TableName() string { return "os_active_config" }
