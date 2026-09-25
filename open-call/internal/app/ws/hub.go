package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"open-call/internal/datetime"
	"open-call/internal/httpapi"
	"open-call/internal/observability"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"open-call/internal/layers/biz/agent"
	"open-call/internal/layers/biz/auth"
	"open-call/internal/ports"
)

type envelope struct {
	Type    string         `json:"type"`
	Seq     uint64         `json:"seq,omitempty"`
	TS      string         `json:"ts,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

type client struct {
	conn      *websocket.Conn
	mu        sync.Mutex
	principal auth.Principal
}

type retainedEvent struct {
	callID, agentID string
	message         envelope
}

func (c *client) send(ctx context.Context, v any) error {
	b, err := datetime.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.conn.Write(writeCtx, websocket.MessageText, b)
}

// Hub 维护 WebSocket 连接并按 agent/call 推送事件。
type Hub struct {
	log            *slog.Logger
	auth           *auth.Service
	calls          ports.CallControlPort
	agents         *agent.Service
	hooks          ports.WebhookDispatcher
	connCount      atomic.Int64
	sequence       atomic.Uint64
	allowedOrigins []string

	mu      sync.Mutex
	byAgent map[string]map[*client]struct{}
	byCall  map[string]map[*client]struct{}
	recent  []retainedEvent
}

// NewHub 创建 WS 网关。
func NewHub(log *slog.Logger, allowedOrigins ...string) *Hub {
	return &Hub{
		log:            log,
		byAgent:        map[string]map[*client]struct{}{},
		byCall:         map[string]map[*client]struct{}{},
		allowedOrigins: allowedOrigins,
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
	ctx = observability.With(ctx, observability.Context{CallID: ev.CallID, AgentID: ev.AgentID})
	observability.Emit(ctx, "ws.call_event.published", map[string]any{"type": ev.Type})
	msg := envelope{Type: ev.Type, Seq: h.sequence.Add(1), TS: datetime.Format(time.Now()), Payload: ev.Payload}
	h.retain(ev.CallID, ev.AgentID, msg)
	h.broadcast(ctx, ev.CallID, ev.AgentID, msg)
	if h.hooks != nil {
		if err := h.hooks.Dispatch(ctx, ev.Type, ev.Payload); err != nil {
			return err
		}
	}
	return nil
}

func (h *Hub) PublishAgentEvent(ctx context.Context, ev ports.AgentEvent) error {
	ctx = observability.With(ctx, observability.Context{AgentID: ev.AgentID})
	observability.Emit(ctx, "ws.agent_event.published", map[string]any{"type": ev.Type})
	msg := envelope{Type: ev.Type, Seq: h.sequence.Add(1), TS: datetime.Format(time.Now()), Payload: ev.Payload}
	h.retain("", ev.AgentID, msg)
	h.broadcast(ctx, "", ev.AgentID, msg)
	if h.hooks != nil {
		if err := h.hooks.Dispatch(ctx, ev.Type, ev.Payload); err != nil {
			return err
		}
	}
	return nil
}

// ServeHTTP 校验来源和令牌，升级连接后接收消息并维护客户端生命周期。
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		httpapi.Failure(w, http.StatusForbidden, "websocket_error", "WebSocket Origin 不允许")
		return
	}
	protocol, token := authProtocol(r.Header.Get("Sec-WebSocket-Protocol"))
	if token == "" {
		httpapi.Failure(w, http.StatusUnauthorized, "websocket_error", "缺少 WebSocket 认证协议")
		return
	}
	if h.auth == nil {
		httpapi.Failure(w, http.StatusServiceUnavailable, "websocket_error", "认证未就绪")
		return
	}
	p, err := h.auth.Authenticate(r.Context(), token)
	if err != nil {
		httpapi.Failure(w, http.StatusUnauthorized, "websocket_error", "未认证或令牌失效")
		return
	}
	sessionID := observability.NewID()
	ctx := observability.With(r.Context(), observability.Context{
		AgentID: p.AgentID, CallID: p.GuestCallID, ClientSessionID: sessionID,
	})
	r = r.WithContext(ctx)

	// Origin 已由 originAllowed 按配置白名单严格校验；关闭库内置的同源复检，
	// 避免反向代理改写 Host 后误拒绝合法的 WebSocket 连接。
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:       []string{protocol},
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.log.Warn("websocket accept failed", "err", err)
		return
	}
	cl := &client{conn: conn, principal: p}
	observability.Emit(ctx, "ws.connected", map[string]any{"role": p.Role})
	h.connCount.Add(1)
	h.add(cl)
	defer func() {
		h.remove(cl)
		if cl.principal.AgentID != "" && h.agents != nil {
			h.mu.Lock()
			remaining := len(h.byAgent[cl.principal.AgentID])
			h.mu.Unlock()
			if remaining == 0 {
				info, err := h.agents.ByID(context.Background(), cl.principal.AgentID)
				if err == nil && info.TerminalType != "sip" && info.State == "idle" {
					_ = h.agents.SetState(context.Background(), info.AgentID, "idle", "busy", "disconnected")
				}
			}
		}
		h.connCount.Add(-1)
		observability.Emit(ctx, "ws.disconnected", map[string]any{"role": p.Role})
		_ = conn.Close(websocket.StatusNormalClosure, "bye")
	}()

	_ = cl.send(r.Context(), envelope{
		Type:    "system.connected",
		TS:      datetime.Format(time.Now()),
		Payload: map[string]any{"role": p.Role},
	})
	if since, err := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64); err == nil && since > 0 {
		h.replay(r.Context(), cl, since)
	}

	ctx, cancel := context.WithDeadline(r.Context(), p.ExpiresAt)
	defer cancel()
	go h.watchAuthentication(ctx, conn, token)
	conn.SetReadLimit(64 * 1024)
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if _, err := h.auth.Authenticate(ctx, token); err != nil {
			_ = conn.Close(websocket.StatusPolicyViolation, "令牌已失效")
			return
		}
		h.handleClient(ctx, cl, data)
	}
}

// retain 将事件放入有界缓冲，供断线客户端按序号补收。
func (h *Hub) retain(callID, agentID string, msg envelope) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recent = append(h.recent, retainedEvent{callID: callID, agentID: agentID, message: msg})
	if len(h.recent) > 1000 {
		h.recent = append([]retainedEvent(nil), h.recent[len(h.recent)-1000:]...)
	}
}

// replay 只补发序号大于 since 且属于当前客户端的事件。
func (h *Hub) replay(ctx context.Context, cl *client, since uint64) {
	h.mu.Lock()
	events := make([]retainedEvent, 0)
	for _, ev := range h.recent {
		if ev.message.Seq <= since {
			continue
		}
		if ev.agentID == cl.principal.AgentID || (ev.callID != "" && ev.callID == cl.principal.GuestCallID) {
			events = append(events, ev)
		}
	}
	h.mu.Unlock()
	for _, ev := range events {
		if err := cl.send(ctx, ev.message); err != nil {
			return
		}
	}
}

func (h *Hub) watchAuthentication(ctx context.Context, conn *websocket.Conn, token string) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := h.auth.Authenticate(ctx, token); err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "令牌已失效")
				return
			}
		}
	}
}

func authProtocol(header string) (string, string) {
	for _, part := range strings.Split(header, ",") {
		p := strings.TrimSpace(part)
		if strings.HasPrefix(p, "open-voip.auth.") {
			return p, strings.TrimPrefix(p, "open-voip.auth.")
		}
	}
	return "", ""
}

func (h *Hub) originAllowed(r *http.Request) bool {
	origin := strings.TrimSuffix(strings.TrimSpace(r.Header.Get("Origin")), "/")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, allowed := range h.allowedOrigins {
		if strings.EqualFold(strings.TrimSuffix(allowed, "/"), origin) {
			return true
		}
	}
	return false
}

func (h *Hub) handleClient(ctx context.Context, cl *client, data []byte) {
	var msg envelope
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	switch msg.Type {
	case "ping":
		_ = cl.send(ctx, envelope{Type: "pong", TS: datetime.Format(time.Now())})
	case "call.answer":
		if h.calls == nil || cl.principal.AgentID == "" {
			return
		}
		callID, _ := msg.Payload["call_id"].(string)
		ctx = observability.With(ctx, observability.Context{CallID: callID, AgentID: cl.principal.AgentID})
		observability.Emit(ctx, "ws.call.answer", nil)
		if err := h.calls.Answer(ctx, callID, cl.principal.AgentID); err != nil {
			_ = cl.send(ctx, envelope{Type: "error", Payload: map[string]any{"message": err.Error()}})
		}
	case "call.decline":
		if h.calls == nil || cl.principal.AgentID == "" {
			return
		}
		callID, _ := msg.Payload["call_id"].(string)
		ctx = observability.With(ctx, observability.Context{CallID: callID, AgentID: cl.principal.AgentID})
		observability.Emit(ctx, "ws.call.decline", nil)
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
		_ = cl.send(ctx, envelope{Type: "error", Payload: map[string]any{"message": "请通过通话 API 响应视频请求"}})
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

// broadcast 按通话或坐席路由消息，避免泄露给无关连接。
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
		if err := c.send(ctx, msg); err != nil {
			h.remove(c)
			_ = c.conn.Close(websocket.StatusGoingAway, "write failed")
		}
	}
}
