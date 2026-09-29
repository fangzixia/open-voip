package models

import "time"

// AgentStateProjection 来自 Switch 事件流的坐席路由状态变更历史（只读投影）。
type AgentStateProjection struct {
	ID        int64 `gorm:"primaryKey;autoIncrement:false"`
	AgentID   string
	FromState string
	ToState   string
	Reason    string
	CreatedAt time.Time
}

func (AgentStateProjection) TableName() string { return "oc_agent_state_projection" }
