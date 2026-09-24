package models

import "time"

// Call 表示一通呼叫中心会话，媒体 Room ID 与此 ID 一致。
type Call struct {
	Caller       string
	Callee       string
	AgentID      string
	OfferedAgent string
	AnsweredAt   *time.Time
	// ID 全局通话 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:通话 ID 与媒体 Room 一致"`
	// Direction 方向：inbound / outbound / internal。
	Direction string `gorm:"size:16;not null;index;comment:呼叫方向"`
	// SessionType 媒介类型：audio / video / mixed。
	SessionType string `gorm:"size:16;not null;default:audio;comment:会话媒介类型"`
	// State FSM 当前状态，由 L3 维护。
	State string `gorm:"size:32;not null;index;comment:呼叫状态机状态"`
	// QueueID 关联队列，可空（外呼等）。
	QueueID *string `gorm:"type:uuid;index;comment:关联队列 ID"`
	// ParentCallID 咨询转等场景的父通话 ID。
	ParentCallID *string `gorm:"type:uuid;index;comment:父通话 ID"`
	// Priority 入队优先级，数值越大越优先。
	Priority int `gorm:"not null;default:0;comment:优先级"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
	// UpdatedAt 最后状态变更时间。
	UpdatedAt time.Time `gorm:"comment:更新时间"`
	// EndedAt 挂断时间，未结束为空。
	EndedAt *time.Time `gorm:"comment:结束时间"`
}

// TableName 指定表名。
func (Call) TableName() string { return "os_calls" }

// CallLeg 表示通话中的一条媒体腿（客户、坐席、IVR 等）。
type CallLeg struct {
	// ID 通话腿 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:通话腿 ID"`
	// CallID 所属通话。
	CallID string `gorm:"type:uuid;index;not null;comment:所属通话 ID"`
	// Role 角色：customer / agent / ivr_bot / supervisor / pstn。
	Role string `gorm:"size:32;not null;comment:腿角色"`
	// AgentID 坐席腿时关联 agents.id，可空。
	AgentID *string `gorm:"type:uuid;index;comment:坐席 ID"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (CallLeg) TableName() string { return "os_call_legs" }
