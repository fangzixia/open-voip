package models

import "time"

// IVRPublishedSnapshot 已发布 IVR 快照，按配置版本存储，呼入只读。
type IVRPublishedSnapshot struct {
	// FlowID 所属流程。
	FlowID string `gorm:"type:uuid;primaryKey;not null;comment:流程 ID"`
	// Version 单调递增版本号。
	Version int `gorm:"index:idx_ivr_flow_version,priority:2,sort:desc;not null;comment:发布版本"`
	// PayloadJSON 已发布节点树 JSON。
	PayloadJSON   string `gorm:"column:payload_json;type:text;not null;comment:发布内容 JSON"`
	ConfigVersion int64  `gorm:"primaryKey;not null"`
	// PublishedAt 发布时间。
	PublishedAt time.Time `gorm:"comment:发布时间"`
}

// TableName 指定表名。
func (IVRPublishedSnapshot) TableName() string { return "os_ivr_published_snapshots" }
