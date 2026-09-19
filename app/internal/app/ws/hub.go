package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"open-voip/internal/layers/biz/agent"
	"open-voip/internal/layers/biz/auth"
	"open-voip/internal/ports"
)

type envelope struct {
	Type    string         `json:"type"`
	TS      string         `json:"ts,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

type client struct {
	conn       *websocket.Conn
	mu         sync.Mutex
	principal  auth.Principal
}

func (c *client) send(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.Write(ctx, websocket.MessageText, b)
}

// Hub 维护 WebSocket 连接并按 agent/call 推送事件。
type Hub struct {
	log       *slog.Logger
	auth      *auth.Service
	calls     ports.CallControlPort
	agents    *agent.Service
	hooks     ports.WebhookDispatcher
	connCount atomic.Int64

	mu      sync.Mutex
	byAgent map[string]map[*client]struct{}
	byCall  map[string]map[*client]struct{}
}

// NewHub 创建 WS 网关。
func NewHub(log *slog.Logger) *Hub {
	return &Hub{
		log:     log,
		byAgent: map[string]map[*client]struct{}{},
		byCall:  map[string]map[*client]struct{}{},
	}
}

// Configure 注入鉴权与呼叫端口（打破组合根循环依赖）。
func (h *Hub) Configure(authSvc *auth.Service, calls ports.CallControlPort, agents *agent.Service, hooks ports.WebhookDispatcher) {
	h.auth = authSvc
	h.calls = calls
	h.agents = agents
	h.hooks = hooks
}

var _ ports.CallEventPublisher = (*Hub)(nil)
var _ ports.AgentEventPublisher = (*Hub)(nil)

// ConnectionCount 当前连接数。
func (h *Hub) ConnectionCount() int {
	return int(h.connCount.Load())
}

func (h *Hub) PublishCallEvent(ctx context.Context, ev ports.CallEvent) error {
	msg := envelope{Type: ev.Type, TS: time.Now().UTC().Format(time.RFC3339), Payload: ev.Payload}
	h.broadcast(ctx, ev.CallID, ev.AgentID, msg)
	if h.hooks != nil {
		_ = h.hooks.Dispatch(ctx, ev.Type, ev.Payload)
	}
	return nil
}

func (h *Hub) PublishAgentEvent(ctx context.Context, ev ports.AgentEvent) error {
	msg := envelope{Type: ev.Type, TS: time.Now().UTC().Format(time.RFC3339), Payload: ev.Payload}
	h.broadcast(ctx, "", ev.AgentID, msg)
	if h.hooks != nil {
		_ = h.hooks.Dispatch(ctx, ev.Type, ev.Payload)
	}
	return nil
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "缺少 token 查询参数", http.StatusUnauthorized)
		return
	}
	if h.auth == nil {
		http.Error(w, "认证未就绪", http.StatusServiceUnavailable)
		return
	}
	p, err := h.auth.Authenticate(r.Context(), token)
	if err != nil {
		http.Error(w, "未认证或令牌失效", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		h.log.Warn("websocket accept failed", "err", err)
		return
	}
	cl := &client{conn: conn, principal: p}
	h.connCount.Add(1)
	h.add(cl)
	defer func() {
		h.remove(cl)
		h.connCount.Add(-1)
		_ = conn.Close(websocket.StatusNormalClosure, "bye")
	}()

	_ = cl.send(r.Context(), envelope{
		Type:    "system.connected",
		TS:      time.Now().UTC().Format(time.RFC3339),
		Payload: map[string]any{"role": p.Role},
	})

	ctx := r.Context()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		h.handleClient(ctx, cl, data)
	}
}

func (h *Hub) handleClient(ctx context.Context, cl *client, data []byte) {
	var msg envelope
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	switch msg.Type {
	case "ping":
		_ = cl.send(ctx, envelope{Type: "pong", TS: time.Now().UTC().Format(time.RFC3339)})
	case "call.answer":
		if h.calls == nil || cl.principal.AgentID == "" {
			return
		}
		callID, _ := msg.Payload["call_id"].(string)
		if err := h.calls.Answer(ctx, callID, cl.principal.AgentID); err != nil {
			_ = cl.send(ctx, envelope{Type: "error", Payload: map[string]any{"message": err.Error()}})
		}
	case "call.decline":
		if h.calls == nil || cl.principal.AgentID == "" {
			return
		}
		callID, _ := msg.Payload["call_id"].(string)
		_ = h.calls.Decline(ctx, callID, cl.principal.AgentID)
	case "agent.set_state":
		if h.agents == nil || cl.principal.AgentID == "" {
			return
		}
		state, _ := msg.Payload["state"].(string)
		reason, _ := msg.Payload["busy_reason"].(string)
		_, err := h.agents.UpdateState(ctx, cl.principal.AgentID, state, reason)
		if err != nil {
			_ = cl.send(ctx, envelope{Type: "error", Payload: map[string]any{"message": err.Error()}})
		}
	case "video.respond":
		if h.calls == nil || msg.Payload == nil {
			return
		}
		callID, _ := msg.Payload["call_id"].(string)
		accept, _ := msg.Payload["accept"].(bool)
		_ = h.calls.RespondVideo(ctx, callID, accept)
	}
}

func (h *Hub) add(cl *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cl.principal.AgentID != "" {
		if h.byAgent[cl.principal.AgentID] == nil {
			h.byAgent[cl.principal.AgentID] = map[*client]struct{}{}
		}
		h.byAgent[cl.principal.AgentID][cl] = struct{}{}
	}
	if cl.principal.GuestCallID != "" {
		if h.byCall[cl.principal.GuestCallID] == nil {
			h.byCall[cl.principal.GuestCallID] = map[*client]struct{}{}
		}
		h.byCall[cl.principal.GuestCallID][cl] = struct{}{}
	}
}

func (h *Hub) remove(cl *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m := h.byAgent[cl.principal.AgentID]; m != nil {
		delete(m, cl)
		if len(m) == 0 {
			delete(h.byAgent, cl.principal.AgentID)
		}
	}
	if m := h.byCall[cl.principal.GuestCallID]; m != nil {
		delete(m, cl)
		if len(m) == 0 {
			delete(h.byCall, cl.principal.GuestCallID)
		}
	}
}

func (h *Hub) broadcast(ctx context.Context, callID, agentID string, msg envelope) {
	h.mu.Lock()
	var targets []*client
	if agentID != "" {
		for c := range h.byAgent[agentID] {
			targets = append(targets, c)
		}
	}
	if callID != "" {
		for c := range h.byCall[callID] {
			targets = append(targets, c)
		}
		// 坐席接听后也按 agent 连接收 ended/answered；振铃已按 agent 推送。
	}
	h.mu.Unlock()
	seen := map[*client]struct{}{}
	for _, c := range targets {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		_ = c.send(ctx, msg)
	}
}
