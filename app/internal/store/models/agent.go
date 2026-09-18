package models

import "time"

// Agent 表示呼叫中心坐席业务属性，与 User 一对一扩展。
type Agent struct {
	// ID 主键 UUID，通常与关联 User.ID 相同或外键关联。
	ID string `gorm:"type:uuid;primaryKey;comment:坐席主键 UUID"`
	// UserID 关联 users.id。
	UserID string `gorm:"type:uuid;uniqueIndex;not null;comment:关联用户 ID"`
	// Extension 分机短号，例如 1001。
	Extension string `gorm:"size:16;uniqueIndex;not null;comment:分机号"`
	// VideoCapable 是否具备视频服务能力。
	VideoCapable bool `gorm:"not null;default:false;comment:是否支持视频"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `gorm:"comment:更新时间"`
}

// TableName 指定表名。
func (Agent) TableName() string { return "agents" }

// Skill 表示技能组，用于队列路由匹配。
type Skill struct {
	// ID 主键 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:技能组 ID"`
	// Name 技能名称，唯一。
	Name string `gorm:"size:64;uniqueIndex;not null;comment:技能名称"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (Skill) TableName() string { return "skills" }

// AgentSkill 坐席与技能多对多关联。
type AgentSkill struct {
	// AgentID 坐席 ID。
	AgentID string `gorm:"type:uuid;primaryKey;comment:坐席 ID"`
	// SkillID 技能 ID。
	SkillID string `gorm:"type:uuid;primaryKey;index;comment:技能 ID"`
}

// TableName 指定表名。
func (AgentSkill) TableName() string { return "agent_skills" }

// AgentSession 表示一次签入会话（队列绑定、WS 关联、当前状态）。
type AgentSession struct {
	// ID 签入会话 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:签入会话 ID"`
	// AgentID 坐席 ID，同一时刻仅允许一条活跃签入。
	AgentID string `gorm:"type:uuid;uniqueIndex;not null;comment:坐席 ID 签入唯一"`
	// State 当前状态：offline idle busy ringing on_call acw 等。
	State string `gorm:"size:32;not null;index;comment:坐席状态"`
	// BusyReason 示忙原因码，空闲时为空。
	BusyReason string `gorm:"size:64;comment:示忙原因"`
	// CheckedInAt 签入时间。
	CheckedInAt time.Time `gorm:"comment:签入时间"`
	// UpdatedAt 状态最后变更时间。
	UpdatedAt time.Time `gorm:"comment:状态更新时间"`
}

// TableName 指定表名。
func (AgentSession) TableName() string { return "agent_sessions" }

// AgentStateLog 坐席状态变更审计，供报表 RPT-04。
type AgentStateLog struct {
	// ID 日志 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:状态日志 ID"`
	// AgentID 坐席 ID。
	AgentID string `gorm:"type:uuid;index;not null;comment:坐席 ID"`
	// FromState 变更前状态。
	FromState string `gorm:"size:32;comment:原状态"`
	// ToState 变更后状态。
	ToState string `gorm:"size:32;not null;comment:新状态"`
	// Reason 变更原因或 busy_reason。
	Reason string `gorm:"size:128;comment:变更原因"`
	// CreatedAt 记录时间。
	CreatedAt time.Time `gorm:"index;comment:记录时间"`
}

// TableName 指定表名。
func (AgentStateLog) TableName() string { return "agent_state_log" }
