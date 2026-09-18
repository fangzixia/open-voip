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
	// Result 结果码。
	Result string
	// StartedAt 开始时间。
	StartedAt time.Time
	// AnsweredAt 接通时间。
	AnsweredAt *time.Time
	// EndedAt 结束时间。
	EndedAt *time.Time
}

// CDRRecorderPort 由 L4 cdr 实现，L3 禁止直接 SQL。
type CDRRecorderPort interface {
	// Upsert 创建或更新话单记录。
	Upsert(ctx context.Context, req CDRWriteRequest) error
}
