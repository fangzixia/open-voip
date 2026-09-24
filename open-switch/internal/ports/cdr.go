package ports

import (
	"context"
	"time"

	"open-switch/internal/ports/dto"
)

// CDRWriteRequest FSM 迁移点写入话单的字段集合。
type CDRWriteRequest struct {
	// CallID 通话 ID。
	CallID string `json:"call_id"`
	// Direction 呼叫方向：inbound / outbound / internal。
	Direction string `json:"direction"`
	// QueueID 队列，可空。
	QueueID string `json:"queue_id"`
	// AgentID 坐席，可空。
	AgentID string `json:"agent_id"`
	// Caller 主叫。
	Caller string `json:"caller"`
	// Callee 被叫。
	Callee string `json:"callee"`
	// SessionType 媒介类型。
	SessionType dto.SessionType `json:"session_type"`
	// Result 结果码 answered / abandoned / failed。
	Result string `json:"result"`
	// StartedAt 开始时间。
	StartedAt time.Time `json:"started_at"`
	// AnsweredAt 接通时间。
	AnsweredAt *time.Time `json:"answered_at"`
	// EndedAt 结束时间。
	EndedAt *time.Time `json:"ended_at"`
	// VideoStartedAt 视频开始时间。
	VideoStartedAt *time.Time `json:"video_started_at"`
	// VideoUpgradeOk 升视频是否成功。
	VideoUpgradeOk bool `json:"video_upgrade_ok"`
	// ScreenShareCount 屏幕共享次数。
	ScreenShareCount int `json:"screen_share_count"`
	// NotifyMessage 录音告知。
	NotifyMessage string `json:"notify_message"`
	// RetainDays 录音保留天数。
	RetainDays int `json:"retain_days"`
}

// CDRRecorderPort 由 L4 cdr 实现，L3 禁止直接 SQL。
type CDRRecorderPort interface {
	// Upsert 创建或更新话单记录。
	Upsert(ctx context.Context, req CDRWriteRequest) error
}
