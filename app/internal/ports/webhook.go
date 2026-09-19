package ports

import "context"

// WebhookDispatcher 由 L4 webhook 实现，app/ws 在发布事件时同步投递。
type WebhookDispatcher interface {
	// Dispatch 按订阅投递事件；失败写入 deliveries 表。
	Dispatch(ctx context.Context, eventType string, payload map[string]any) error
}
