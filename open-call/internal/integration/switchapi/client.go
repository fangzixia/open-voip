// Package switchapi 通过 HTTP 调用 open-switch Switch API。
package switchapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"open-call/internal/datetime"
	"open-call/internal/httpapi"
	"open-call/internal/observability"
	"strings"
	"time"

	"open-call/internal/config"
	"open-call/internal/ports"
	"open-call/internal/ports/dto"
)

// Client 实现 CallControlPort，供 guest 与 WebSocket 使用。
type Client struct {
	base   string
	secret string
	http   *http.Client
}

// NewClient 创建 Switch HTTP 客户端。
func NewClient(cfg config.IntegrationConfig) *Client {
	return &Client{
		base:   strings.TrimRight(cfg.SwitchBaseURL, "/"),
		secret: cfg.Secret,
		http:   &http.Client{Timeout: 30 * time.Second},
	}
}

// do 统一发送 Switch API 请求，透传调用方身份和追踪字段并解析错误响应。
func (c *Client) do(ctx context.Context, method, path string, in any, principalJSON string, out any) error {
	var body io.Reader
	if in != nil {
		b, err := datetime.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Content-Type", "application/json")
	if principalJSON != "" {
		req.Header.Set("X-Principal", principalJSON)
	}
	setTraceHeaders(req, ctx)
	observability.Emit(ctx, "switch.request.started", map[string]any{"method": method, "path": path})
	res, err := c.http.Do(req)
	if err != nil {
		observability.Emit(ctx, "switch.request.failed", map[string]any{"method": method, "path": path, "error": err.Error()})
		return err
	}
	defer func() { _ = res.Body.Close() }()
	err = httpapi.Decode(res, out)
	fields := map[string]any{"method": method, "path": path, "status": res.StatusCode}
	if err != nil {
		fields["error"] = err.Error()
		observability.Emit(ctx, "switch.request.failed", fields)
		return err
	}
	observability.Emit(ctx, "switch.request.completed", fields)
	return nil
}

func setTraceHeaders(req *http.Request, ctx context.Context) {
	ids := observability.From(ctx)
	req.Header.Set("X-Request-ID", httpapi.ID(ctx))
	if ids.TraceID != "" {
		req.Header.Set("X-Trace-ID", ids.TraceID)
	}
	if ids.CallID != "" {
		req.Header.Set("X-Call-ID", ids.CallID)
	}
	if ids.LegID != "" {
		req.Header.Set("X-Leg-ID", ids.LegID)
	}
	if ids.AgentID != "" {
		req.Header.Set("X-Agent-ID", ids.AgentID)
	}
	if ids.QueueID != "" {
		req.Header.Set("X-Queue-ID", ids.QueueID)
	}
	if ids.ClientSessionID != "" {
		req.Header.Set("X-Client-Session-ID", ids.ClientSessionID)
	}
}

func (c *Client) StartInbound(ctx context.Context, req dto.InboundRequest) (string, error) {
	var out ports.CallView
	if err := c.do(ctx, http.MethodPost, "/switch/v1/calls/inbound", req, "", &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

func (c *Client) GetCall(ctx context.Context, callID string) (ports.CallView, error) {
	var out ports.CallView
	err := c.do(ctx, http.MethodGet, "/switch/v1/internal/calls/"+callID, nil, "", &out)
	return out, err
}

func (c *Client) Answer(ctx context.Context, callID, agentID string) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls/"+callID+"/answer", nil, principalAgent(agentID), nil)
}

func (c *Client) Decline(ctx context.Context, callID, agentID string) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls/"+callID+"/decline", nil, principalAgent(agentID), nil)
}

func (c *Client) Hangup(ctx context.Context, callID string, reason dto.HangupReason) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls/"+callID+"/hangup", map[string]string{"reason": string(reason)}, "", nil)
}

func (c *Client) Transfer(ctx context.Context, callID string, req dto.TransferRequest) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls/"+callID+"/transfer", req, "", nil)
}

func (c *Client) CompleteTransfer(ctx context.Context, callID string) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls/"+callID+"/transfer/complete", nil, "", nil)
}

func (c *Client) Outbound(ctx context.Context, req dto.OutboundRequest) (string, error) {
	var out ports.CallView
	if err := c.do(ctx, http.MethodPost, "/switch/v1/calls/outbound", req, principalAgent(req.AgentID), &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

func (c *Client) StartIVR(ctx context.Context, callID, snapshotID string) error {
	return fmt.Errorf("StartIVR: 请经 Switch API 扩展")
}

func (c *Client) Hold(ctx context.Context, callID string, on bool) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls/"+callID+"/hold", map[string]bool{"on": on}, "", nil)
}

func (c *Client) RequestVideo(ctx context.Context, callID, fromLegID string) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls/"+callID+"/video/request", nil, "", nil)
}

func (c *Client) RespondVideo(ctx context.Context, callID string, accept bool) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls/"+callID+"/video/respond", map[string]bool{"accept": accept}, "", nil)
}

func (c *Client) DowngradeVideo(ctx context.Context, callID string) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls/"+callID+"/video/downgrade", nil, "", nil)
}

func (c *Client) ScreenShare(ctx context.Context, callID, legID string, on bool) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls/"+callID+"/screen-share", map[string]any{"on": on, "leg_id": legID}, "", nil)
}

func (c *Client) ConferenceInvite(ctx context.Context, callID, targetAgentID string) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls/"+callID+"/conference", map[string]string{"agent_id": targetAgentID}, "", nil)
}

func (c *Client) SupervisorListen(ctx context.Context, callID, supervisorAgentID string) (string, error) {
	var out map[string]string
	if err := c.do(ctx, http.MethodPost, "/switch/v1/supervisor/calls/"+callID+"/listen", nil, principalAgent(supervisorAgentID), &out); err != nil {
		return "", err
	}
	return out["leg_id"], nil
}

func (c *Client) SendDTMF(ctx context.Context, callID, legID, digit string) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls/"+callID+"/dtmf", map[string]string{"leg_id": legID, "digit": digit}, "", nil)
}

func (c *Client) ForceReleaseAgent(ctx context.Context, agentID, policy string) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/supervisor/agents/"+agentID+"/force-check-out", map[string]string{"policy": policy}, "", nil)
}

func principalAgent(agentID string) string {
	if agentID == "" {
		return ""
	}
	b, _ := json.Marshal(map[string]string{"agent_id": agentID, "role": "agent"})
	return string(b)
}

var _ ports.CallControlPort = (*Client)(nil)
