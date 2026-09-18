package models

import "time"

// User 表示可登录系统的账号（管理员、班长或坐席）。
type User struct {
	// ID 主键 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:用户主键 UUID"`
	// Username 登录名，全局唯一。
	Username string `gorm:"size:64;uniqueIndex;not null;comment:登录用户名"`
	// PasswordHash argon2id 编码后的密码哈希。
	PasswordHash string `gorm:"size:255;not null;comment:argon2id 密码哈希"`
	// Role 角色：admin / supervisor / agent。
	Role string `gorm:"size:32;not null;index;comment:角色 admin supervisor agent"`
	// DisplayName 展示名称，可空。
	DisplayName string `gorm:"size:128;comment:展示名"`
	// Disabled 为 true 时禁止登录。
	Disabled bool `gorm:"not null;default:false;comment:是否禁用账号"`
	// CreatedAt 创建时间 UTC。
	CreatedAt time.Time `gorm:"comment:创建时间"`
	// UpdatedAt 更新时间 UTC。
	UpdatedAt time.Time `gorm:"comment:更新时间"`
}

// TableName 指定表名。
func (User) TableName() string { return "users" }
