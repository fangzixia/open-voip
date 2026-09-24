package models

import "time"

// DIDRoute 外显/DID 号码到队列的路由（MEDIA-08）。
type DIDRoute struct {
	// ID 路由 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:DID 路由 ID"`
	// DID 被叫号码或外显。
	DID string `gorm:"column:d_id;size:32;uniqueIndex;not null;comment:DID 号码"`
	// QueueID 目标队列。
	QueueID string `gorm:"type:uuid;index;not null;comment:目标队列 ID"`
	// DisplayName 外显名称，可空。
	DisplayName string `gorm:"size:128;comment:外显名称"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `gorm:"comment:更新时间"`
}

// TableName 指定表名。
func (DIDRoute) TableName() string { return "oc_did_routes" }
