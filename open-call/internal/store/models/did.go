package models

import "time"

// DIDRoute 遗留 DID 草稿模型（运行时以 Switch 为准；configio 导出仍用此结构）。
type DIDRoute struct {
	// ID 路由 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:DID 路由 ID"`
	// DID 被叫号码或外显。
	DID string `gorm:"column:d_id;size:32;uniqueIndex;not null;comment:DID 号码"`
	// TrunkID 中继范围；* 表示全部中继。
	TrunkID string `gorm:"size:64;not null;default:*;comment:中继 ID"`
	// TargetType 目标类型：queue（队列）/ ivr / reject（拒接）。
	TargetType string `gorm:"size:16;not null;comment:目标类型"`
	// TargetID 队列或 IVR 流程 ID；reject 时为空。
	TargetID *string `gorm:"type:uuid;comment:目标 ID"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `gorm:"comment:更新时间"`
}

// TableName 指定表名。
func (DIDRoute) TableName() string { return "oc_did_routes" }
