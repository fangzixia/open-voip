package ports

import (
	"context"
	"time"
)

// ConfigBundle is the complete immutable call-centre configuration for one application.
type ConfigBundle struct {
	Version int64         `json:"version,omitempty"`
	Queues  []QueueConfig `json:"queues"`
	Skills  []SkillConfig `json:"skills"`
	Agents  []AgentConfig `json:"agents"`
	DIDs    []DIDConfig   `json:"dids"`
	IVRs    []IVRConfig   `json:"ivrs"`
}

type QueueConfig struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	VideoEnabled          bool     `json:"video_enabled"`
	MaxWaitSec            int      `json:"max_wait_sec"`
	DispatchStrategy      string   `json:"dispatch_strategy"`
	RecordingPolicy       string   `json:"recording_policy"`
	OverflowAction        string   `json:"overflow_action"`
	OverflowQueueID       string   `json:"overflow_queue_id,omitempty"`
	IVRFlowID             string   `json:"ivr_flow_id,omitempty"`
	WaitPrompt            string   `json:"wait_prompt,omitempty"`
	AnnounceRecording     bool     `json:"announce_recording"`
	PriorityEnabled       bool     `json:"priority_enabled"`
	BusinessHoursJSON     string   `json:"business_hours_json,omitempty"`
	AfterHoursAction      string   `json:"after_hours_action"`
	ForceHangupOnCheckout bool     `json:"force_hangup_on_checkout"`
	ListenAnnounce        bool     `json:"listen_announce"`
	SkillIDs              []string `json:"skill_ids,omitempty"`
	AgentIDs              []string `json:"agent_ids,omitempty"`
}

type SkillConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AgentConfig struct {
	ID           string   `json:"id"`
	UserRef      string   `json:"user_ref"`
	Extension    string   `json:"extension"`
	DisplayName  string   `json:"display_name,omitempty"`
	VideoCapable bool     `json:"video_capable"`
	TerminalType string   `json:"terminal_type"`
	SIPUsername  string   `json:"sip_username,omitempty"`
	Enabled      bool     `json:"enabled"`
	SkillIDs     []string `json:"skill_ids,omitempty"`
}

type DIDConfig struct {
	ID         string `json:"id"`
	TrunkID    string `json:"trunk_id"`
	DID        string `json:"did"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id,omitempty"`
}

type IVRConfig struct {
	FlowID      string `json:"flow_id"`
	Version     int    `json:"version"`
	PayloadJSON string `json:"payload_json"`
}

type ConfigVersionView struct {
	ApplicationID string     `json:"application_id"`
	Version       int64      `json:"version"`
	Status        string     `json:"status"`
	Checksum      string     `json:"checksum"`
	CreatedAt     time.Time  `json:"created_at"`
	ActivatedAt   *time.Time `json:"activated_at,omitempty"`
}

type AgentSessionView struct {
	AgentID       string   `json:"agent_id"`
	State         string   `json:"state"`
	BusyReason    string   `json:"busy_reason,omitempty"`
	CurrentCallID string   `json:"current_call_id,omitempty"`
	QueueIDs      []string `json:"queue_ids"`
}

type QueueStatusView struct {
	QueueID         string `json:"queue_id"`
	Waiting         int64  `json:"waiting"`
	AvailableAgents int64  `json:"available_agents"`
	RingingAgents   int64  `json:"ringing_agents"`
	BusyAgents      int64  `json:"busy_agents"`
}

// CallCenterAdminPort is the only mutable configuration and agent-presence surface.
type CallCenterAdminPort interface {
	StoreConfig(context.Context, ConfigBundle) (ConfigVersionView, error)
	ActivateConfig(context.Context, int64) (ConfigVersionView, error)
	GetConfigVersion(context.Context, int64) (ConfigVersionView, error)
	CheckIn(context.Context, string, []string) (AgentSessionView, error)
	CheckOut(context.Context, string) error
	SetPresence(context.Context, string, string, string) (AgentSessionView, error)
	AgentSession(context.Context, string) (AgentSessionView, error)
	QueueStatus(context.Context, string) (QueueStatusView, error)
}
