package models

import "time"

// IVRFlow IVR 流程定义（草稿与元数据）。
type IVRFlow struct {
	// ID 流程 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:IVR 流程 ID"`
	// Name 流程名称。
	Name string `gorm:"size:128;not null;comment:流程名称"`
	// DraftJSON 草稿配置 JSON。
	DraftJSON string `gorm:"type:text;comment:草稿 JSON 配置"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `gorm:"comment:更新时间"`
}

// TableName 指定表名。
func (IVRFlow) TableName() string { return "os_ivr_flows" }

// IVRPublishedSnapshot 已发布 IVR 快照，呼入只读最新 version。
type IVRPublishedSnapshot struct {
	// ID 快照 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:快照 ID"`
	// FlowID 所属流程。
	FlowID string `gorm:"type:uuid;index:idx_ivr_flow_version,priority:1;not null;comment:流程 ID"`
	// Version 单调递增版本号。
	Version int `gorm:"index:idx_ivr_flow_version,priority:2,sort:desc;not null;comment:发布版本"`
	// PayloadJSON 已发布节点树 JSON。
	PayloadJSON string `gorm:"type:text;not null;comment:发布内容 JSON"`
	// PublishedAt 发布时间。
	PublishedAt time.Time `gorm:"comment:发布时间"`
}

// TableName 指定表名。
func (IVRPublishedSnapshot) TableName() string { return "os_ivr_published_snapshots" }
