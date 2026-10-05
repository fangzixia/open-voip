package ports

import (
	"context"
	"time"
)

type SwitchConfigBundle struct {
	Version int64               `json:"version,omitempty"`
	Queues  []SwitchQueueConfig `json:"queues"`
	Skills  []SwitchSkillConfig `json:"skills"`
	Agents  []SwitchAgentConfig `json:"agents"`
	DIDs    []SwitchDIDConfig   `json:"dids"`
	IVRs    []SwitchIVRConfig   `json:"ivrs"`
}

type SwitchQueueConfig struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	VideoEnabled          bool     `json:"video_enabled"`
	MaxWaitSec            int      `json:"max_wait_sec"`
	DispatchStrategy      string   `json:"dispatch_strategy"`
	RecordingPolicy       string   `json:"recording_policy"`
	OverflowAction        string   `json:"overflow_action"`
	OverflowQueueID       string   `json:"overflow_queue_id,omitempty"`
	IVRFlowID             string   `json:"ivr_flow_id,omitempty"`
	PostCallIVRFlowID     string   `json:"post_call_ivr_flow_id,omitempty"`
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

type SwitchSkillConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type SwitchAgentConfig struct {
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
type SwitchDIDConfig struct {
	ID         string `json:"id"`
	TrunkID    string `json:"trunk_id"`
	DID        string `json:"did"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id,omitempty"`
}
type SwitchIVRConfig struct {
	FlowID      string `json:"flow_id"`
	PayloadJSON string `json:"payload_json"`
	Version     int    `json:"version"`
}
type SwitchConfigVersion struct {
	Version     int64      `json:"version"`
	Status      string     `json:"status"`
	Checksum    string     `json:"checksum"`
	CreatedAt   time.Time  `json:"created_at"`
	ActivatedAt *time.Time `json:"activated_at,omitempty"`
}
type SwitchAgentSession struct {
	AgentID       string   `json:"agent_id"`
	State         string   `json:"state"`
	BusyReason    string   `json:"busy_reason,omitempty"`
	CurrentCallID string   `json:"current_call_id,omitempty"`
	QueueIDs      []string `json:"queue_ids"`
}

type SwitchActiveConfiguration struct {
	Version SwitchConfigVersion `json:"version"`
	Bundle  SwitchConfigBundle  `json:"bundle"`
}

type SwitchIVRPublishedView struct {
	FlowID      string `json:"flow_id"`
	Version     int    `json:"version"`
	PayloadJSON string `json:"payload_json"`
}

// SwitchIVRVersionView 与已发布视图同形（兼容旧命名）。
type SwitchIVRVersionView = SwitchIVRPublishedView

// SwitchAdminPort 是 Switch 管理面的完整能力集合；只用到部分能力的调用方应依赖下面的窄接口。
type SwitchAdminPort interface {
	SwitchConfigVersionPort
	SwitchQueueConfigPort
	SwitchSkillConfigPort
	SwitchAgentConfigPort
	SwitchDIDConfigPort
	SwitchIVRFlowPort
	SwitchAgentRuntimePort
}

// SwitchConfigVersionPort 整包配置版本的存储、激活与读取。
type SwitchConfigVersionPort interface {
	StoreConfig(context.Context, SwitchConfigBundle) (SwitchConfigVersion, error)
	ActivateConfig(context.Context, int64) (SwitchConfigVersion, error)
	GetActiveConfiguration(context.Context) (SwitchActiveConfiguration, error)
	GetActiveConfigurationSummary(context.Context) (SwitchConfigVersion, error)
}

// SwitchQueueConfigPort 队列配置。
type SwitchQueueConfigPort interface {
	ListQueueConfigs(context.Context) ([]SwitchQueueConfig, error)
	CreateQueueConfig(context.Context, SwitchQueueConfig) (SwitchQueueConfig, error)
	GetQueueConfig(context.Context, string) (SwitchQueueConfig, error)
	UpdateQueueConfig(context.Context, string, SwitchQueueConfig) (SwitchQueueConfig, error)
	DeleteQueueConfig(context.Context, string) error
	SetQueueAgents(context.Context, string, []string) (SwitchQueueConfig, error)
	SetQueueSkills(context.Context, string, []string) (SwitchQueueConfig, error)
}

// SwitchSkillConfigPort 技能配置。
type SwitchSkillConfigPort interface {
	ListSkillConfigs(context.Context) ([]SwitchSkillConfig, error)
	CreateSkillConfig(context.Context, SwitchSkillConfig) (SwitchSkillConfig, error)
	UpdateSkillConfig(context.Context, string, string) (SwitchSkillConfig, error)
	DeleteSkillConfig(context.Context, string) error
}

// SwitchAgentConfigPort 坐席路由配置（非运行时状态）。
type SwitchAgentConfigPort interface {
	ListAgentConfigs(context.Context) ([]SwitchAgentConfig, error)
	UpsertAgentConfig(context.Context, SwitchAgentConfig) (SwitchAgentConfig, error)
	DeleteAgentConfig(context.Context, string) error
	SetAgentSkills(context.Context, string, []string) (SwitchAgentConfig, error)
}

// SwitchDIDConfigPort DID 路由配置。
type SwitchDIDConfigPort interface {
	ListDIDConfigs(context.Context) ([]SwitchDIDConfig, error)
	UpsertDIDConfig(context.Context, SwitchDIDConfig) (SwitchDIDConfig, error)
	DeleteDIDConfig(context.Context, string) error
}

// SwitchIVRFlowPort 已发布 IVR 流程。
type SwitchIVRFlowPort interface {
	ListIVRFlows(context.Context) ([]SwitchIVRPublishedView, error)
	GetIVRFlow(context.Context, string) (SwitchIVRPublishedView, error)
	ValidateIVRFlowPayload(context.Context, string) error
	UpsertIVRFlow(context.Context, string, string) (SwitchIVRPublishedView, error)
	DeleteIVRFlow(context.Context, string) error
}

// SwitchAgentRuntimePort 坐席签入签出与在线状态。
type SwitchAgentRuntimePort interface {
	CheckIn(context.Context, string, []string) (SwitchAgentSession, error)
	CheckOut(context.Context, string) error
	SetPresence(context.Context, string, string, string) (SwitchAgentSession, error)
	AgentSession(context.Context, string) (SwitchAgentSession, error)
	// ListAgentSessions 批量返回已签入坐席会话；未返回的坐席即 offline。
	ListAgentSessions(context.Context) ([]SwitchAgentSession, error)
}
