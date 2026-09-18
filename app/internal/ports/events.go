package ports

import "context"

// CallEvent 呼叫类 WebSocket 事件载荷，类型字符串见 docs/events.md。
type CallEvent struct {
	// Type 例如 call.ringing、call.answered。
	Type string
	// CallID 通话 ID。
	CallID string
	// Payload 业务 JSON 对象，由序列化层编码。
	Payload map[string]any
}

// AgentEvent 坐席类 WebSocket 事件。
type AgentEvent struct {
	// Type 例如 agent.state_changed。
	Type string
	// AgentID 坐席 ID。
	AgentID string
	// Payload 扩展字段。
	Payload map[string]any
}

// CallEventPublisher 由 app/ws 实现，L3 注入发布 call.* / video.*。
type CallEventPublisher interface {
	PublishCallEvent(ctx context.Context, ev CallEvent) error
}

// AgentEventPublisher 由 app/ws 实现，L4 注入发布 agent.*。
type AgentEventPublisher interface {
	PublishAgentEvent(ctx context.Context, ev AgentEvent) error
}
