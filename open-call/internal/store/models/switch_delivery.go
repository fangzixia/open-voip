package models

import "time"

// SwitchEventInbox 记录已从 Switch 拉取并提交的事件，用于去重与审计。
type SwitchEventInbox struct {
	EventID       int64 `gorm:"primaryKey"`
	ApplicationID string
	ReceivedAt    time.Time
}

func (SwitchEventInbox) TableName() string { return "oc_switch_event_inbox" }

// SwitchEventOutbox 待向 WebSocket/Webhook 发布的序列化呼叫事件。
type SwitchEventOutbox struct {
	EventID     int64 `gorm:"primaryKey"`
	Payload     string
	DeliveredAt *time.Time
}

func (SwitchEventOutbox) TableName() string { return "oc_switch_event_outbox" }
