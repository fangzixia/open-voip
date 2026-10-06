package models

import "time"

// AibotUsage AI 虚拟坐席会话用量（客户/回复音频毫秒）。
type AibotUsage struct {
	ID            string    `gorm:"primaryKey;size:36"`
	CallID        string    `gorm:"size:36;index;not null"`
	AgentID       string    `gorm:"size:36;index"`
	QueueID       string    `gorm:"size:36;index"`
	InputAudioMs  int64     `gorm:"not null"`
	OutputAudioMs int64     `gorm:"not null"`
	StartedAt     time.Time `gorm:"not null"`
	EndedAt       time.Time `gorm:"not null"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
}

func (AibotUsage) TableName() string { return "oc_aibot_usage" }

// AibotQueueProfile 队列级 AI 提示词（管理端配置）。
type AibotQueueProfile struct {
	ID           string    `gorm:"primaryKey;size:36"`
	QueueID      string    `gorm:"size:36;uniqueIndex;not null"`
	SystemPrompt string    `gorm:"type:text;not null"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}

func (AibotQueueProfile) TableName() string { return "oc_aibot_queue_profiles" }
