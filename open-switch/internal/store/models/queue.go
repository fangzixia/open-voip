package models

import "time"

// Queue 呼入队列配置。
type Queue struct {
	// ID 队列 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:队列 ID"`
	// Name 队列名称。
	Name string `gorm:"size:128;not null;comment:队列名称"`
	// VideoEnabled 是否为视频队列。
	VideoEnabled bool `gorm:"not null;default:false;comment:是否视频队列"`
	// MaxWaitSec 最大排队等待秒数，0 表示不限制。
	MaxWaitSec int `gorm:"not null;default:300;comment:最大等待秒数"`
	// DispatchStrategy 分配策略：longest_idle / round_robin。
	DispatchStrategy string `gorm:"size:32;not null;default:longest_idle;comment:ACD 策略"`
	// RecordingPolicy 录音策略：off / audio / video_composite。
	RecordingPolicy string `gorm:"size:32;not null;default:off;comment:录音策略"`
	// OverflowAction 溢出动作：hangup / voicemail / queue。
	OverflowAction string `gorm:"size:32;comment:溢出动作"`
	// OverflowQueueID 溢出目标队列，可空。
	OverflowQueueID *string `gorm:"type:uuid;comment:溢出目标队列 ID"`
	// IVRFlowID 绑定的 IVR 流程，可空。
	IVRFlowID *string `gorm:"type:uuid;index;comment:绑定 IVR 流程 ID"`
	// WaitPrompt 排队文案模板，可含 {position}。
	WaitPrompt string `gorm:"size:256;comment:排队提示文案"`
	// AnnounceRecording 入队/接通前是否告知录音。
	AnnounceRecording bool `gorm:"not null;default:false;comment:是否播放录音告知"`
	// PriorityEnabled 是否启用优先级入队。
	PriorityEnabled bool `gorm:"not null;default:false;comment:是否优先级队列"`
	// BusinessHoursJSON 工作时间 JSON，空表示 always。
	BusinessHoursJSON string `gorm:"type:text;comment:工作时间 JSON"`
	// AfterHoursAction 非工作时间动作：hangup / voicemail。
	AfterHoursAction string `gorm:"size:32;comment:非工作时间动作"`
	// ForceHangupOnCheckout 班长强制签出时是否立即挂断。
	ForceHangupOnCheckout bool `gorm:"not null;default:true;comment:强制签出是否挂断"`
	// ListenAnnounce 班长监听时是否向客户播放提示音。
	ListenAnnounce bool `gorm:"not null;default:false;comment:监听是否提示客户"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `gorm:"comment:更新时间"`
}

// QueueSkill 队列所需技能。
type QueueSkill struct {
	// QueueID 队列 ID。
	QueueID string `gorm:"type:uuid;primaryKey;comment:队列 ID"`
	// SkillID 技能 ID。
	SkillID string `gorm:"type:uuid;primaryKey;index;comment:技能 ID"`
}

// TableName 指定表名。
func (QueueSkill) TableName() string { return "os_queue_skills" }

// TableName 指定表名。
func (Queue) TableName() string { return "os_queues" }

// QueueAgent 队列与可签入坐席的绑定关系。
type QueueAgent struct {
	// QueueID 队列 ID。
	QueueID string `gorm:"type:uuid;primaryKey;comment:队列 ID"`
	// AgentID 坐席 ID。
	AgentID string `gorm:"type:uuid;primaryKey;index;comment:坐席 ID"`
}

// TableName 指定表名。
func (QueueAgent) TableName() string { return "os_queue_agents" }
