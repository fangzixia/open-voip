package models

import "time"

// DIDRoute 外显/DID 号码到队列的路由（MEDIA-08）。
type DIDRoute struct {
	// ID 路由 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:DID 路由 ID"`
	// DID 被叫号码或外显。
	DID           string  `gorm:"column:normalized_did;size:32;not null;comment:DID 号码"`
	TrunkID       string  `gorm:"not null;default:'*'"`
	TargetType    string  `gorm:"not null"`
	TargetID      *string `gorm:"type:uuid"`
	ConfigVersion int64   `gorm:"primaryKey;not null"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `gorm:"comment:更新时间"`
}

// TableName 指定表名。
func (DIDRoute) TableName() string { return "os_did_routes" }
