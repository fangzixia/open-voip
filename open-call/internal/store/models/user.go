package models

import "time"

// User 表示可登录系统的账号（管理员、班长或坐席）。
type User struct {
	// ID 主键 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:用户主键 UUID"`
	// Username 登录名，全局唯一。
	Username string `gorm:"size:64;uniqueIndex;not null;comment:登录用户名"`
	// Email 用于识别需要管理员显式绑定的外部同邮箱账号。
	Email string `gorm:"size:320;not null;default:''"`
	// PasswordHash argon2id 编码后的密码哈希。
	PasswordHash string `gorm:"size:255;not null;comment:argon2id 密码哈希"`
	// Role 角色：admin / supervisor / agent。
	Role string `gorm:"size:32;not null;index;comment:角色 admin supervisor agent"`
	// DisplayName 展示名称，可空。
	DisplayName string `gorm:"size:128;comment:展示名"`
	// Disabled 为 true 时禁止登录。
	Disabled bool `gorm:"not null;default:false;comment:是否禁用账号"`
	// AuthVersion 在密码、角色或禁用状态变化后递增，使既有令牌立即失效。
	AuthVersion int64 `gorm:"not null;default:1;comment:认证版本"`
	// MustChangePassword 要求用户下次登录后先修改临时密码。
	MustChangePassword bool `gorm:"not null;default:false;comment:是否强制修改密码"`
	// CreatedAt 创建时间 UTC。
	CreatedAt time.Time `gorm:"comment:创建时间"`
	// UpdatedAt 更新时间 UTC。
	UpdatedAt time.Time `gorm:"comment:更新时间"`
}

// TableName 指定表名。
func (User) TableName() string { return "oc_users" }

// AuthSession 表示一个可单独撤销的登录会话。
type AuthSession struct {
	// ID 登录会话主键。
	ID string `gorm:"type:uuid;primaryKey"`
	// UserID 关联用户。
	UserID string `gorm:"type:uuid;not null;index"`
	// RefreshJTI 当前刷新令牌标识，令牌轮换时同步更新。
	RefreshJTI string `gorm:"size:64;not null;uniqueIndex"`
	// UserAgent 登录客户端信息。
	UserAgent string `gorm:"size:256"`
	// RemoteIP 登录来源地址。
	RemoteIP string `gorm:"size:64"`
	// CreatedAt 会话创建时间。
	CreatedAt time.Time `gorm:"not null"`
	// ExpiresAt 会话绝对过期时间。
	ExpiresAt time.Time `gorm:"not null;index"`
	// LastSeenAt 最近刷新时间。
	LastSeenAt time.Time `gorm:"not null"`
	// RevokedAt 会话撤销时间，为空表示仍有效。
	RevokedAt            *time.Time `gorm:"index"`
	Provider             string     `gorm:"size:16;not null;default:local"`
	ProviderRefreshToken string     `gorm:"type:text"`
	ProviderSubject      string     `gorm:"size:255"`
}

// TableName 返回登录会话表名。
func (AuthSession) TableName() string { return "oc_auth_sessions" }
