package ports

import "context"

// AgentInfo 坐席目录条目，供 L3 振铃与视频能力判断。
type AgentInfo struct {
	// TerminalType 坐席终端：webrtc 或 sip。
	TerminalType string `json:"terminal_type"`
	// SIPUsername 对应 Switch 配置中的设备账号，不包含密码。
	SIPUsername string `json:"sip_username,omitempty"`
	// AgentID 坐席 UUID。
	AgentID string `json:"agent_id"`
	// UserID 关联用户 ID。
	UserID string `json:"user_id"`
	// Extension 分机号。
	Extension string `json:"extension"`
	// VideoCapable 是否支持视频。
	VideoCapable bool `json:"video_capable"`
	// DisplayName 展示名。
	DisplayName string `json:"display_name"`
	// State 当前签入状态，未签入为空或 offline。
	State string `json:"state"`
}

// AgentDirectoryPort 由 L4 agent 包实现，L3 解析分机与能力并更新振铃/通话状态。
type AgentDirectoryPort interface {
	// ByExtension 根据分机号查询坐席。
	ByExtension(ctx context.Context, extension string) (AgentInfo, error)
	// ByID 根据坐席 ID 查询。
	ByID(ctx context.Context, agentID string) (AgentInfo, error)
	// SetState 乐观锁更新坐席状态：仅当当前状态等于 fromState 时迁移到 toState。
	// fromState 为空则不校验原状态。成功后应写入状态日志并发布 agent.state_changed。
	SetState(ctx context.Context, agentID, fromState, toState, reason string) error
}
