package ports

import (
	"context"
	"time"
)

// ConfigBundle 单个应用的完整、不可变呼叫中心配置快照。
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

// ActiveConfigurationView 当前激活的完整配置快照。
type ActiveConfigurationView struct {
	Version ConfigVersionView `json:"version"`
	Bundle  ConfigBundle      `json:"bundle"`
}

// IVRFlowView IVR 草稿视图。
type IVRFlowView struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	DraftJSON        string    `json:"draft_json"`
	PublishedVersion int       `json:"published_version,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// IVRVersionView 已发布 IVR 版本。
type IVRVersionView struct {
	FlowID      string    `json:"flow_id"`
	Version     int       `json:"version"`
	PayloadJSON string    `json:"payload_json"`
	PublishedAt time.Time `json:"published_at"`
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

// CallCenterAdminPort 唯一可变的配置与坐席在线状态管理接口。
type CallCenterAdminPort interface {
	StoreConfig(context.Context, ConfigBundle) (ConfigVersionView, error)
	ActivateConfig(context.Context, int64) (ConfigVersionView, error)
	GetConfigVersion(context.Context, int64) (ConfigVersionView, error)
	GetActiveConfiguration(context.Context) (ActiveConfigurationView, error)
	GetActiveConfigurationSummary(context.Context) (ConfigVersionView, error)

	ListQueueConfigs(context.Context) ([]QueueConfig, error)
	CreateQueueConfig(context.Context, QueueConfig) (QueueConfig, error)
	GetQueueConfig(context.Context, string) (QueueConfig, error)
	UpdateQueueConfig(context.Context, string, QueueConfig) (QueueConfig, error)
	DeleteQueueConfig(context.Context, string) error
	SetQueueAgents(context.Context, string, []string) (QueueConfig, error)
	SetQueueSkills(context.Context, string, []string) (QueueConfig, error)

	ListSkillConfigs(context.Context) ([]SkillConfig, error)
	CreateSkillConfig(context.Context, SkillConfig) (SkillConfig, error)
	UpdateSkillConfig(context.Context, string, string) (SkillConfig, error)
	DeleteSkillConfig(context.Context, string) error

	ListAgentConfigs(context.Context) ([]AgentConfig, error)
	UpsertAgentConfig(context.Context, AgentConfig) (AgentConfig, error)
	DeleteAgentConfig(context.Context, string) error
	SetAgentSkills(context.Context, string, []string) (AgentConfig, error)

	ListDIDConfigs(context.Context) ([]DIDConfig, error)
	UpsertDIDConfig(context.Context, DIDConfig) (DIDConfig, error)
	DeleteDIDConfig(context.Context, string) error

	ListIVRFlows(context.Context) ([]IVRFlowView, error)
	CreateIVRFlow(context.Context, string, string) (IVRFlowView, error)
	GetIVRFlow(context.Context, string) (IVRFlowView, error)
	UpdateIVRFlow(context.Context, string, string, string) (IVRFlowView, error)
	DeleteIVRFlow(context.Context, string) error
	PublishIVRFlow(context.Context, string) (IVRVersionView, error)
	ListIVRVersions(context.Context, string) ([]IVRVersionView, error)
	RollbackIVRFlow(context.Context, string, int) (IVRVersionView, error)

	CheckIn(context.Context, string, []string) (AgentSessionView, error)
	CheckOut(context.Context, string) error
	SetPresence(context.Context, string, string, string) (AgentSessionView, error)
	AgentSession(context.Context, string) (AgentSessionView, error)
	QueueStatus(context.Context, string) (QueueStatusView, error)
}
