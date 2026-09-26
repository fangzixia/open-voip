// Package platform 通过 HTTP 调用 open-call Platform API，实现 L3 所需的 L4 Port。
package platform

import (
	"bytes"
	"context"
	"gorm.io/gorm"
	"io"
	"net/http"
	"open-switch/internal/datetime"
	"open-switch/internal/httpapi"
	"open-switch/internal/observability"
	"strings"
	"time"

	"net/url"
	"open-switch/internal/config"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

// Client 调用业务系统 Platform API。
type Client struct {
	outbox *gorm.DB
	base   string
	secret string
	http   *http.Client
}

// NewClient 创建 Platform HTTP 客户端。
func NewClient(cfg config.IntegrationConfig) *Client {
	return &Client{
		base:   strings.TrimRight(cfg.PlatformBaseURL, "/"),
		secret: cfg.Secret,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) auth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Content-Type", "application/json")
}

func (c *Client) doJSON(ctx context.Context, method, path string, in, out any) error {
	started := time.Now()
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
	c.auth(req)
	req.Header.Set("X-Request-ID", httpapi.ID(ctx))
	req.Header.Set("X-Trace-ID", httpapi.TraceID(ctx))
	res, err := c.http.Do(req)
	if err != nil {
		observability.Event(ctx, "platform_client", "platform.http", "response", "error", "transport_error", started, "method", method, "path", path, "error", err)
		return err
	}
	defer func() { _ = res.Body.Close() }()
	err = httpapi.Decode(res, out)
	result, reason := "ok", ""
	if err != nil {
		result, reason = "error", "upstream_error"
	}
	observability.Event(ctx, "platform_client", "platform.http", "response", result, reason, started, "method", method, "path", path, "status", res.StatusCode)
	return err
}

// RequestAgent 实现 ACDDispatchPort。
func (c *Client) RequestAgent(ctx context.Context, req dto.DispatchRequest) (dto.DispatchResult, error) {
	var out dto.DispatchResult
	err := c.doJSON(ctx, http.MethodPost, "/platform/v1/acd/dispatch", req, &out)
	return out, err
}

// ByExtension 实现 AgentDirectoryPort。
func (c *Client) ByExtension(ctx context.Context, extension string) (ports.AgentInfo, error) {
	var out ports.AgentInfo
	err := c.doJSON(ctx, http.MethodGet, "/platform/v1/agents/by-extension/"+url.PathEscape(extension), nil, &out)
	return out, err
}

// ByID 实现 AgentDirectoryPort。
func (c *Client) ByID(ctx context.Context, agentID string) (ports.AgentInfo, error) {
	var out ports.AgentInfo
	err := c.doJSON(ctx, http.MethodGet, "/platform/v1/agents/"+agentID, nil, &out)
	return out, err
}

// SetState 实现 AgentDirectoryPort。
func (c *Client) SetState(ctx context.Context, agentID, fromState, toState, reason string) error {
	body := map[string]string{
		"from_state": fromState,
		"to_state":   toState,
		"reason":     reason,
	}
	return c.doJSON(ctx, http.MethodPost, "/platform/v1/agents/"+agentID+"/state", body, nil)
}

// GetQueue 实现 ConfigSnapshotPort。
func (c *Client) GetQueue(ctx context.Context, queueID string) (ports.QueueSnapshot, error) {
	var out ports.QueueSnapshot
	err := c.doJSON(ctx, http.MethodGet, "/platform/v1/queues/"+queueID+"/snapshot", nil, &out)
	return out, err
}

// GetLatestIVR 实现 ConfigSnapshotPort。
func (c *Client) GetLatestIVR(ctx context.Context, flowID string) (ports.IVRSnapshot, error) {
	var out ports.IVRSnapshot
	err := c.doJSON(ctx, http.MethodGet, "/platform/v1/ivr/flows/"+flowID+"/snapshot/latest", nil, &out)
	return out, err
}

// GetBusinessHours 实现 ConfigSnapshotPort。
func (c *Client) GetBusinessHours(ctx context.Context, queueID string) (ports.BusinessHours, error) {
	var out ports.BusinessHours
	path := "/platform/v1/queues/" + queueID + "/business-hours"
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// ResolveDID 实现 ConfigSnapshotPort。
func (c *Client) ResolveDID(ctx context.Context, did string) (string, error) {
	var out struct {
		QueueID string `json:"queue_id"`
	}
	err := c.doJSON(ctx, http.MethodGet, "/platform/v1/dids/resolve?did="+url.QueryEscape(did), nil, &out)
	return out.QueueID, err
}

// Now 实现 ConfigSnapshotPort。
func (c *Client) Now(ctx context.Context) time.Time {
	return time.Now().UTC()
}

// ForQueue 实现 RecordingPolicyPort。
func (c *Client) ForQueue(ctx context.Context, queueID string) (dto.RecordingPolicy, error) {
	var out dto.RecordingPolicy
	err := c.doJSON(ctx, http.MethodGet, "/platform/v1/queues/"+queueID+"/recording-policy", nil, &out)
	return out, err
}

// Upsert 实现 CDRRecorderPort。
func (c *Client) Upsert(ctx context.Context, req ports.CDRWriteRequest) error {
	return c.enqueue(ctx, "/platform/v1/cdr", req)
}

// Save 实现 RecordingStorePort。
func (c *Client) Save(ctx context.Context, rec ports.RecordingMeta) error {
	return c.enqueue(ctx, "/platform/v1/recordings", rec)
}

// EventPublisher 将 call.* 事件推送到 open-call。
type EventPublisher struct {
	client *Client
}

// NewEventPublisher 创建事件推送器。
func NewEventPublisher(c *Client) *EventPublisher {
	return &EventPublisher{client: c}
}

// PublishCallEvent 实现 CallEventPublisher。
func (p *EventPublisher) PublishCallEvent(ctx context.Context, ev ports.CallEvent) error {
	body := map[string]any{
		"event_id": ev.ID,
		"seq":      ev.Seq,
		"type":     ev.Type,
		"call_id":  ev.CallID,
		"agent_id": ev.AgentID,
		"payload":  ev.Payload,
	}
	return p.client.enqueue(ctx, "/platform/v1/events/call", body)
}
