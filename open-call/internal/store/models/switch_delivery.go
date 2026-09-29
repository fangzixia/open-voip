package models

import "time"

type SwitchEventInbox struct {
	EventID       int64 `gorm:"primaryKey"`
	ApplicationID string
	ReceivedAt    time.Time
}

func (SwitchEventInbox) TableName() string { return "oc_switch_event_inbox" }

type SwitchEventOutbox struct {
	EventID     int64 `gorm:"primaryKey"`
	Payload     string
	DeliveredAt *time.Time
}

func (SwitchEventOutbox) TableName() string { return "oc_switch_event_outbox" }
