package ports

import (
	"context"
	"time"
)

// CommandView 异步命令查询结果（方案 §6 可恢复命令）。
type CommandView struct {
	ID             string         `json:"id"`
	CallID         string         `json:"call_id,omitempty"`
	Type           string         `json:"type"`
	Status         string         `json:"status"`
	Result         map[string]any `json:"result"`
	ErrorCode      string         `json:"error_code,omitempty"`
	IdempotencyKey string         `json:"idempotency_key"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// CommandStore 持久化异步媒体/拨号命令并支持幂等重放。
type CommandStore interface {
	Accept(ctx context.Context, callID, idempotencyKey, requestHash, typ string, result map[string]any) (CommandView, bool, error)
	MarkRunning(ctx context.Context, id string) error
	Complete(ctx context.Context, id, status, errCode string, result map[string]any) error
	Get(ctx context.Context, id string) (CommandView, error)
	ReconcileStale(ctx context.Context, olderThan time.Duration) error
}
