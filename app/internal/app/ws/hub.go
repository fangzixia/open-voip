// Package ws 实现 L1 WebSocket 网关，并作为 Call/Agent 事件发布器注入 L3/L4。
package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/coder/websocket"

	"open-voip/internal/ports"
)

// Hub 维护 WebSocket 连接并在 Phase 1 实现按 agent/call 订阅推送。
type Hub struct {
	log *slog.Logger
	// originPatterns WebSocket Origin 白名单，与 cors.allowed_origins 一致；["*"] 仅建议开发一体部署。
	originPatterns []string
	// connCount 当前连接数，供 status API。
	connCount atomic.Int64
}

// NewHub 创建 WS 网关。originPatterns 为空时在 Accept 阶段拒绝跨域升级。
func NewHub(log *slog.Logger, originPatterns []string) *Hub {
	patterns := append([]string(nil), originPatterns...)
	return &Hub{log: log, originPatterns: patterns}
}

var _ ports.CallEventPublisher = (*Hub)(nil)
var _ ports.AgentEventPublisher = (*Hub)(nil)

// ConnectionCount 返回当前 WebSocket 连接数。
func (h *Hub) ConnectionCount() int {
	return int(h.connCount.Load())
}

// PublishCallEvent Phase 0 仅记录日志；Phase 1 按订阅推送。
func (h *Hub) PublishCallEvent(ctx context.Context, ev ports.CallEvent) error {
	h.log.Debug("call event", "type", ev.Type, "call_id", ev.CallID)
	return nil
}

// PublishAgentEvent Phase 0 仅记录日志。
func (h *Hub) PublishAgentEvent(ctx context.Context, ev ports.AgentEvent) error {
	h.log.Debug("agent event", "type", ev.Type, "agent_id", ev.AgentID)
	return nil
}

// ServeHTTP 处理 WebSocket 升级；Phase 0 接受连接并维持心跳占位。
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "缺少 token 查询参数", http.StatusUnauthorized)
		return
	}

	patterns := h.originPatterns
	if len(patterns) == 0 {
		http.Error(w, "WebSocket 未配置允许的 Origin", http.StatusForbidden)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: patterns,
	})
	if err != nil {
		h.log.Warn("websocket accept failed", "err", err)
		return
	}

	h.connCount.Add(1)
	defer func() {
		h.connCount.Add(-1)
		_ = conn.Close(websocket.StatusNormalClosure, "bye")
	}()

	_ = conn.Write(r.Context(), websocket.MessageText, mustJSON(map[string]string{
		"type":    "system.connected",
		"message": "Phase 0：WebSocket 通道已建立，事件推送将在 Phase 1 启用",
	}))

	ctx := r.Context()
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			return
		}
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{"type":"system.error"}`)
	}
	return b
}
