package ports

import (
	"context"
	"time"

	"open-voip/internal/ports/dto"
)

// CDRWriteRequest FSM 迁移点写入话单的字段集合。
type CDRWriteRequest struct {
	// CallID 通话 ID。
	CallID string
	// Direction inbound / outbound / internal。
	Direction string
	// QueueID 队列，可空。
	QueueID string
	// AgentID 坐席，可空。
	AgentID string
	// Caller 主叫。
	Caller string
	// Callee 被叫。
	Callee string
	// SessionType 媒介类型。
	SessionType dto.SessionType
	// Result 结果码 answered / abandoned / failed。
	Result string
	// StartedAt 开始时间。
	StartedAt time.Time
	// AnsweredAt 接通时间。
	AnsweredAt *time.Time
	// EndedAt 结束时间。
	EndedAt *time.Time
	// VideoStartedAt 视频开始时间。
	VideoStartedAt *time.Time
	// VideoUpgradeOk 升视频是否成功。
	VideoUpgradeOk bool
	// ScreenShareCount 屏幕共享次数。
	ScreenShareCount int
	// NotifyMessage 录音告知。
	NotifyMessage string
	// RetainDays 录音保留天数。
	RetainDays int
}

// CDRRecorderPort 由 L4 cdr 实现，L3 禁止直接 SQL。
type CDRRecorderPort interface {
	// Upsert 创建或更新话单记录。
	Upsert(ctx context.Context, req CDRWriteRequest) error
}
