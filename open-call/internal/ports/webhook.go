package ports

import "context"

// WebhookDispatcher 由 L4 webhook 实现，app/ws 只负责持久化投递任务。
type WebhookDispatcher interface {
	// Dispatch 按订阅将事件可靠写入 deliveries 表。
	Dispatch(ctx context.Context, eventType string, payload map[string]any) error
}
