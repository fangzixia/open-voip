package models

import "time"

// GuestSession 访客入会 token 与会话元数据。
type GuestSession struct {
	// ID 会话 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:访客会话 ID"`
	// QueueID 目标队列。
	QueueID string `gorm:"type:uuid;index;not null;comment:目标队列 ID"`
	// CallID 入队后关联的通话，可空。
	CallID *string `gorm:"type:uuid;index;comment:关联通话 ID"`
	// AllowedMedia 允许的媒体：audio / video。
	AllowedMedia string `gorm:"size:16;not null;default:audio;comment:允许媒介"`
	// Priority 由受信任的签发方设置，访客不能自行提升。
	Priority int `gorm:"not null;default:0;comment:队列优先级"`
	// Token 入会 token 哈希或明文（内网 Demo 可简化）。
	Token string `gorm:"size:128;uniqueIndex;not null;comment:入会 token"`
	// ExpiresAt 过期时间 UTC。
	ExpiresAt time.Time `gorm:"index;comment:过期时间"`
	// ConsumedAt 首次入队时间；同一 token 后续调用仅返回原通话。
	ConsumedAt *time.Time `gorm:"comment:首次消费时间"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (GuestSession) TableName() string { return "oc_guest_sessions" }
