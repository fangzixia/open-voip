package models

import "time"

// VoiceNotification is business state; its initiator never occupies a media leg.
type VoiceNotification struct {
	ID          string     `gorm:"primaryKey;size:36" json:"id"`
	InitiatorID string     `json:"-"`
	RequestKey  string     `json:"-"`
	Destination string     `json:"destination"`
	TrunkID     string     `json:"trunk_id"`
	AssetID     string     `json:"asset_id"`
	State       string     `json:"state"`
	LegID       string     `json:"leg_id,omitempty"`
	PlaybackID  string     `json:"playback_id,omitempty"`
	LastError   string     `json:"error,omitempty"`
	Deadline    time.Time  `json:"deadline"`
	LockedUntil *time.Time `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (VoiceNotification) TableName() string { return "oc_voice_notifications" }
