package models

import "time"

// Agent 表示呼叫中心坐席业务属性，与 User 一对一扩展。
type Agent struct {
	// TerminalType 坐席终端类型。
	TerminalType string `gorm:"not null;default:webrtc;comment:终端类型 webrtc 或 sip"`
	// SIPUsername Switch 中的设备账号。
	SIPUsername string `gorm:"column:sip_username;comment:SIP 设备账号"`
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
func (Agent) TableName() string { return "oc_agents" }

// Skill 配置导出/导入 JSON 结构（持久化在 Switch，open-call 无对应表）。
type Skill struct {
	// ID 主键 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:技能组 ID"`
	// Name 技能名称，唯一。
	Name string `gorm:"size:64;uniqueIndex;not null;comment:技能名称"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
}

// TableName 保留 GORM 表名以兼容旧迁移引用。
func (Skill) TableName() string { return "oc_skills" }

// AgentSkill 坐席与技能多对多关联。
type AgentSkill struct {
	// AgentID 坐席 ID。
	AgentID string `gorm:"type:uuid;primaryKey;comment:坐席 ID"`
	// SkillID 技能 ID。
	SkillID string `gorm:"type:uuid;primaryKey;index;comment:技能 ID"`
}

// TableName 指定表名。
func (AgentSkill) TableName() string { return "oc_agent_skills" }
