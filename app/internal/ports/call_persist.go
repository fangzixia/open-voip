package ports

import (
	"context"
	"time"

	"open-voip/internal/ports/dto"
)

// CallRecord 跨层通话持久化记录，禁止引用 GORM model。
type CallRecord struct {
	// ID 通话 UUID，与媒体 Room 一致。
	ID string
	// Direction inbound / outbound / internal。
	Direction string
	// SessionType 媒介类型。
	SessionType dto.SessionType
	// State FSM 状态。
	State string
	// QueueID 关联队列，可空。
	QueueID *string
	// ParentCallID 父通话，可空。
	ParentCallID *string
	// Priority 优先级。
	Priority int
	// CreatedAt 创建时间。
	CreatedAt time.Time
	// UpdatedAt 更新时间。
	UpdatedAt time.Time
	// EndedAt 结束时间，未结束为空。
	EndedAt *time.Time
}

// CallLegRecord 通话腿持久化记录。
type CallLegRecord struct {
	// ID 腿 UUID。
	ID string
	// CallID 所属通话。
	CallID string
	// Role 腿角色。
	Role dto.LegRole
	// AgentID 坐席腿关联，可空。
	AgentID *string
	// CreatedAt 创建时间。
	CreatedAt time.Time
}

// CallPersistencePort 由 store 适配器实现，供 L3 持久化 Call/CallLeg；L3 禁止直接 SQL。
type CallPersistencePort interface {
	// InsertCall 插入新通话。
	InsertCall(ctx context.Context, rec CallRecord) error
	// UpdateCall 按 ID 更新状态与时间戳。
	UpdateCall(ctx context.Context, rec CallRecord) error
	// GetCall 按 ID 读取；不存在返回 errs.ErrNotFound。
	GetCall(ctx context.Context, callID string) (CallRecord, error)
	// InsertLeg 插入通话腿。
	InsertLeg(ctx context.Context, rec CallLegRecord) error
	// ListLegs 列出通话下全部腿。
	ListLegs(ctx context.Context, callID string) ([]CallLegRecord, error)
	// GetLeg 按通话与腿 ID 读取。
	GetLeg(ctx context.Context, callID, legID string) (CallLegRecord, error)
}
