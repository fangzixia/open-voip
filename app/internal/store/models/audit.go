package models

import "time"

// AuditLog 管理操作与敏感访问审计。
type AuditLog struct {
	// ID 日志 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:审计日志 ID"`
	// UserID 操作人，可空（系统动作）。
	UserID *string `gorm:"type:uuid;index;comment:操作人用户 ID"`
	// Action 动作标识，例如 login / config_change。
	Action string `gorm:"size:64;not null;index;comment:动作类型"`
	// Resource 资源类型与 ID 描述。
	Resource string `gorm:"size:256;comment:资源描述"`
	// DetailJSON 详情 JSON。
	DetailJSON string `gorm:"type:text;comment:详情 JSON"`
	// CreatedAt 发生时间。
	CreatedAt time.Time `gorm:"index;comment:发生时间"`
}

// TableName 指定表名。
func (AuditLog) TableName() string { return "audit_logs" }

// JWTRevocation 已撤销 JWT 的 jti 黑名单。
type JWTRevocation struct {
	// JTI JWT ID。
	JTI string `gorm:"size:64;primaryKey;comment:JWT jti"`
	// ExpiresAt 令牌原过期时间，便于清理。
	ExpiresAt time.Time `gorm:"index;comment:令牌过期时间"`
	// RevokedAt 撤销时间。
	RevokedAt time.Time `gorm:"comment:撤销时间"`
}

// TableName 指定表名。
func (JWTRevocation) TableName() string { return "jwt_revocations" }
