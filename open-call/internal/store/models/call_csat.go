package models

import "time"

// CallCsat 满意度评分（由 Switch call.csat_scored 事件投影）。
type CallCsat struct {
	ID       string    `gorm:"type:uuid;primaryKey"`
	CallID   string    `gorm:"type:uuid;uniqueIndex;not null"`
	AgentID  *string   `gorm:"type:uuid"`
	FlowID   *string   `gorm:"type:uuid"`
	Score    int       `gorm:"not null"`
	ScoredAt time.Time `gorm:"not null"`
}

func (CallCsat) TableName() string { return "oc_call_csat" }
