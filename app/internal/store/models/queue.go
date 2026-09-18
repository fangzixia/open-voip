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
	// OverflowAction 溢出动作，例如 hangup / voicemail。
	OverflowAction string `gorm:"size:32;comment:溢出动作"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `gorm:"comment:更新时间"`
}

// TableName 指定表名。
func (Queue) TableName() string { return "queues" }

// QueueAgent 队列与可签入坐席的绑定关系。
type QueueAgent struct {
	// QueueID 队列 ID。
	QueueID string `gorm:"type:uuid;primaryKey;comment:队列 ID"`
	// AgentID 坐席 ID。
	AgentID string `gorm:"type:uuid;primaryKey;index;comment:坐席 ID"`
}

// TableName 指定表名。
func (QueueAgent) TableName() string { return "queue_agents" }
