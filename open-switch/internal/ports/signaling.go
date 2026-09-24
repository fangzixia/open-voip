package ports

import (
	"context"
	"time"

	"open-switch/internal/ports/dto"
)

// CallView 供 HTTP 返回的通话视图。
type CallView struct {
	CreatedAt    time.Time `json:"created_at"`
	Caller       string    `json:"caller"`
	TerminalType string    `json:"terminal_type,omitempty"`
	// ID 通话 ID。
	ID string `json:"id"`
	// State FSM 状态。
	State string `json:"state"`
	// Direction 呼叫方向。
	Direction string `json:"direction"`
	// SessionType 媒介类型。
	SessionType dto.SessionType `json:"session_type"`
	// QueueID 队列，可空。
	QueueID string `json:"queue_id,omitempty"`
	// AgentID 当前振铃或接听坐席，可空。
	AgentID string `json:"agent_id,omitempty"`
	// Held 是否保持。
	Held bool `json:"held,omitempty"`
	// RecordingNotice 录音告知文案，可空。
	RecordingNotice string `json:"recording_notice,omitempty"`
	// Legs 通话腿列表。
	Legs []LegView `json:"legs"`
}

// LegView 通话腿视图。
type LegView struct {
	// ID 腿 ID。
	ID string `json:"id"`
	// Role 角色。
	Role dto.LegRole `json:"role"`
	// AgentID 坐席 ID，可空。
	AgentID string `json:"agent_id,omitempty"`
}

// SignalingPort 由 L3 实现，供 app/http/media 调用；禁止 HTTP 直接持有 MediaPort。
type SignalingPort interface {
	// GetCall 读取通话与腿。
	GetCall(ctx context.Context, callID string) (CallView, error)
	// JoinWebRTC 为腿创建 WebRTC 端并返回服务端 Offer。
	JoinWebRTC(ctx context.Context, callID, legID string) (dto.LocalOffer, error)
	// AcceptAnswer 设置客户端 Answer SDP。
	AcceptAnswer(ctx context.Context, callID, legID, answerSDP string) error
	// TrickleICE 追加客户端 ICE candidate。
	TrickleICE(ctx context.Context, callID, legID string, cand dto.ICECandidateInit) error
	// SetTrackMuted 静音或取消静音。
	SetTrackMuted(ctx context.Context, callID, legID string, audio, video bool) error
	// IssueTURNCredentials 为通话签发短期 TURN 凭证。
	IssueTURNCredentials(ctx context.Context, callID, subject string) (dto.TURNConfig, error)
}
