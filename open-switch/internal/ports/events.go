package ports

import "context"

// CallEvent 呼叫类 WebSocket 事件载荷，类型字符串见 docs/events.md。
type CallEvent struct {
	ID         int64 `json:"id,omitempty"`
	Seq        int64 `json:"seq,omitempty"`
	TargetOnly bool  `json:"target_only,omitempty"`
	// Type 例如 call.ringing、call.answered。
	Type string `json:"type"`
	// CallID 通话 ID。
	CallID string `json:"call_id"`
	// AgentID 可选，用于将振铃事件路由到指定坐席连接。
	AgentID string `json:"agent_id"`
	// Payload 业务 JSON 对象，由序列化层编码。
	Payload map[string]any `json:"payload"`
}

// AgentEvent 坐席类 WebSocket 事件。
type AgentEvent struct {
	// Type 例如 agent.state_changed。
	Type string `json:"type"`
	// AgentID 坐席 ID。
	AgentID string `json:"agent_id"`
	// Payload 扩展字段。
	Payload map[string]any `json:"payload"`
}

// CallEventPublisher 由 app/ws 实现，L3 注入发布 call.* / video.*。
type CallEventPublisher interface {
	PublishCallEvent(ctx context.Context, ev CallEvent) error
}

// AgentEventPublisher 由 app/ws 实现，L4 注入发布 agent.*。
type AgentEventPublisher interface {
	PublishAgentEvent(ctx context.Context, ev AgentEvent) error
}
