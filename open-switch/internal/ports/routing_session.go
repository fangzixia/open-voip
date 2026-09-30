package ports

import (
	"context"
	"time"
)

// RoutingSessionView 路由会话与队列项只读快照。
type RoutingSessionView struct {
	CallID          string     `json:"call_id"`
	State           string     `json:"state"`
	QueueID         string     `json:"queue_id,omitempty"`
	ConfigVersion   int64      `json:"config_version"`
	QueueEntryState string     `json:"queue_entry_state,omitempty"`
	EnqueuedAt      *time.Time `json:"enqueued_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// RoutingSessionPort 读取 os_routing_sessions / os_queue_entries（崩溃恢复与 HTTP 查询）。
type RoutingSessionPort interface {
	Get(ctx context.Context, callID string) (RoutingSessionView, error)
}
