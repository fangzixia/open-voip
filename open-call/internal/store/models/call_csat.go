package models

import "time"

// CallCsat 业务侧解释通用 IVR 输入后生成的满意度评分。
type CallCsat struct {
	ID       string    `gorm:"type:uuid;primaryKey"`
	CallID   string    `gorm:"type:uuid;uniqueIndex;not null"`
	AgentID  *string   `gorm:"type:uuid"`
	FlowID   *string   `gorm:"type:uuid"`
	Score    int       `gorm:"not null"`
	ScoredAt time.Time `gorm:"not null"`
}

func (CallCsat) TableName() string { return "oc_call_csat" }
