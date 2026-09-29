package models

import "time"

// AgentStateProjection is read-only history received from the Switch event stream.
type AgentStateProjection struct {
	ID        int64 `gorm:"primaryKey;autoIncrement:false"`
	AgentID   string
	FromState string
	ToState   string
	Reason    string
	CreatedAt time.Time
}

func (AgentStateProjection) TableName() string { return "oc_agent_state_projection" }
