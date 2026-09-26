// 本文件负责指标和追踪初始化。
// Package observability 提供关联字段、结构化追踪事件和敏感信息脱敏。
package observability

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	TraceIDKey    = "trace_id"
	RequestIDKey  = "request_id"
	CallIDKey     = "call_id"
	LegIDKey      = "leg_id"
	AgentIDKey    = "agent_id"
	QueueIDKey    = "queue_id"
	ComponentKey  = "component"
	EventKey      = "event"
	PhaseKey      = "phase"
	DurationMSKey = "duration_ms"
	ResultKey     = "result"
	ReasonCodeKey = "reason_code"
)

type contextKey string

const fieldsKey contextKey = "observability.fields"

// Fields 定义与 open-call 共用的稳定关联字段。
type Fields struct {
	TraceID, RequestID, CallID, LegID, AgentID, QueueID string
}

func NewID() string { return uuid.NewString() }

func WithFields(ctx context.Context, add Fields) context.Context {
	cur := FromContext(ctx)
	if add.TraceID != "" {
		cur.TraceID = add.TraceID
	}
	if add.RequestID != "" {
		cur.RequestID = add.RequestID
	}
	if add.CallID != "" {
		cur.CallID = add.CallID
	}
	if add.LegID != "" {
		cur.LegID = add.LegID
	}
	if add.AgentID != "" {
		cur.AgentID = add.AgentID
	}
	if add.QueueID != "" {
		cur.QueueID = add.QueueID
	}
	return context.WithValue(ctx, fieldsKey, cur)
}

func FromContext(ctx context.Context) Fields {
	if ctx == nil {
		return Fields{}
	}
	v, _ := ctx.Value(fieldsKey).(Fields)
	return v
}

func Attrs(ctx context.Context) []any {
	f := FromContext(ctx)
	out := make([]any, 0, 12)
	add := func(k, v string) {
		if v != "" {
			out = append(out, k, v)
		}
	}
	add(TraceIDKey, f.TraceID)
	add(RequestIDKey, f.RequestID)
	add(CallIDKey, f.CallID)
	add(LegIDKey, f.LegID)
	add(AgentIDKey, f.AgentID)
	add(QueueIDKey, f.QueueID)
	return out
}

// Event 按固定事件结构输出一条追踪记录。
func Event(ctx context.Context, component, event, phase, result, reason string, started time.Time, attrs ...any) {
	args := Attrs(ctx)
	args = append(args, ComponentKey, component, EventKey, event)
	if phase != "" {
		args = append(args, PhaseKey, phase)
	}
	if !started.IsZero() {
		args = append(args, DurationMSKey, time.Since(started).Milliseconds())
	}
	if result != "" {
		args = append(args, ResultKey, result)
	}
	if reason != "" {
		args = append(args, ReasonCodeKey, reason)
	}
	args = append(args, attrs...)
	slog.InfoContext(ctx, event, args...)
}

var (
	headerSecret   = regexp.MustCompile(`(?im)^((?:proxy-)?authorization|cookie|set-cookie)\s*:\s*[^\r\n]*`)
	bearerSecret   = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	digestResponse = regexp.MustCompile(`(?i)(response\s*=\s*)("[^"]*"|[^,\s]+)`)
	keyValueSecret = regexp.MustCompile(`(?i)\b(password|passwd|credential|turn_credential|auth_secret)\b(\s*[:=]\s*)("[^"]*"|'[^']*'|[^,\s;&]+)`)
	icePwd         = regexp.MustCompile(`(?im)^(a=ice-pwd:)[^\r\n]*`)
)

// Redact 移除凭据并保留排查问题所需的协议结构。
func Redact(s string) string {
	s = headerSecret.ReplaceAllString(s, "$1: [REDACTED]")
	s = bearerSecret.ReplaceAllString(s, "Bearer [REDACTED]")
	s = digestResponse.ReplaceAllString(s, "${1}\"[REDACTED]\"")
	s = keyValueSecret.ReplaceAllString(s, "${1}${2}[REDACTED]")
	s = icePwd.ReplaceAllString(s, "${1}[REDACTED]")
	return s
}

func SensitiveKey(key string) bool {
	k := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	switch k {
	case "authorization", "proxy_authorization", "cookie", "set_cookie", "password", "passwd",
		"credential", "turn_credential", "auth_secret", "sip_digest_response":
		return true
	}
	return strings.Contains(k, "password") || strings.Contains(k, "credential")
}
