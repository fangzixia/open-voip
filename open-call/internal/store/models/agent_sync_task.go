package models

import "time"

// AgentSyncTask 坐席配置同步到 Switch 的重试任务。
type AgentSyncTask struct {
	ID          string    `gorm:"type:uuid;primaryKey"`
	AgentID     string    `gorm:"type:uuid;not null;index"`
	Attempts    int       `gorm:"not null;default:0"`
	NextRetryAt time.Time `gorm:"not null;index"`
	LastError   string    `gorm:"type:text;not null;default:''"`
	CreatedAt   time.Time
}

func (AgentSyncTask) TableName() string { return "oc_agent_sync_tasks" }
