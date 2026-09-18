package models

import "time"

// GuestSession 访客入会 token 与会话元数据。
type GuestSession struct {
	// ID 会话 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:访客会话 ID"`
	// QueueID 目标队列。
	QueueID string `gorm:"type:uuid;index;not null;comment:目标队列 ID"`
	// Token 入会 token 哈希或明文（内网 Demo 可简化）。
	Token string `gorm:"size:128;uniqueIndex;not null;comment:入会 token"`
	// ExpiresAt 过期时间 UTC。
	ExpiresAt time.Time `gorm:"index;comment:过期时间"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (GuestSession) TableName() string { return "guest_sessions" }
