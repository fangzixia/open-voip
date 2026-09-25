// Package observability provides lightweight, structured end-to-end tracing.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"
)

// Context contains correlation identifiers shared by HTTP, WebSocket and service calls.
type Context struct {
	TraceID         string `json:"trace_id,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
	CallID          string `json:"call_id,omitempty"`
	LegID           string `json:"leg_id,omitempty"`
	AgentID         string `json:"agent_id,omitempty"`
	QueueID         string `json:"queue_id,omitempty"`
	ClientSessionID string `json:"client_session_id,omitempty"`
}

type contextKey struct{}

// With stores tracing fields in ctx. Non-empty values override existing fields.
func With(ctx context.Context, fields Context) context.Context {
	current := From(ctx)
	merge(&current, fields)
	return context.WithValue(ctx, contextKey{}, current)
}

// From returns tracing fields from ctx.
func From(ctx context.Context) Context {
	if ctx == nil {
		return Context{}
	}
	value, _ := ctx.Value(contextKey{}).(Context)
	return value
}

func merge(dst *Context, src Context) {
	if src.TraceID != "" {
		dst.TraceID = src.TraceID
	}
	if src.RequestID != "" {
		dst.RequestID = src.RequestID
	}
	if src.CallID != "" {
		dst.CallID = src.CallID
	}
	if src.LegID != "" {
		dst.LegID = src.LegID
	}
	if src.AgentID != "" {
		dst.AgentID = src.AgentID
	}
	if src.QueueID != "" {
		dst.QueueID = src.QueueID
	}
	if src.ClientSessionID != "" {
		dst.ClientSessionID = src.ClientSessionID
	}
}

// NewID returns a cryptographically random 128-bit lowercase hexadecimal ID.
func NewID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
}

// NormalizeID accepts conservative printable correlation IDs and rejects oversized/untrusted values.
func NormalizeID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r == '.' || r == ':' ||
			r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return ""
		}
	}
	return value
}

// Event is one trace JSONL record.
type Event struct {
	Timestamp       string         `json:"timestamp"`
	Name            string         `json:"event"`
	Level           string         `json:"level"`
	Source          string         `json:"source,omitempty"`
	TraceID         string         `json:"trace_id,omitempty"`
	RequestID       string         `json:"request_id,omitempty"`
	CallID          string         `json:"call_id,omitempty"`
	LegID           string         `json:"leg_id,omitempty"`
	AgentID         string         `json:"agent_id,omitempty"`
	QueueID         string         `json:"queue_id,omitempty"`
	ClientSessionID string         `json:"client_session_id,omitempty"`
	Fields          map[string]any `json:"fields,omitempty"`
}

// Recorder serializes trace events as JSONL.
type Recorder struct {
	mu sync.Mutex
	w  io.Writer
}

func NewRecorder(w io.Writer) *Recorder { return &Recorder{w: w} }

// Write emits one sanitized event.
func (r *Recorder) Write(ctx context.Context, name string, fields map[string]any) error {
	if r == nil || r.w == nil {
		return nil
	}
	ids := From(ctx)
	event := Event{
		Timestamp: time.Now().UTC().Format("2006-01-02 15:04:05"),
		Name:      strings.TrimSpace(name), Level: "info", Source: "open-call",
		TraceID: ids.TraceID, RequestID: ids.RequestID, CallID: ids.CallID,
		LegID: ids.LegID, AgentID: ids.AgentID, QueueID: ids.QueueID,
		ClientSessionID: ids.ClientSessionID, Fields: SanitizeMap(fields),
	}
	if event.Name == "" {
		event.Name = "unknown"
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err = r.w.Write(append(raw, '\n'))
	return err
}

var global struct {
	sync.RWMutex
	recorder *Recorder
}

// SetRecorder configures the process-wide trace sink.
func SetRecorder(recorder *Recorder) {
	global.Lock()
	global.recorder = recorder
	global.Unlock()
}

// Emit writes to the process-wide trace sink.
func Emit(ctx context.Context, name string, fields map[string]any) {
	global.RLock()
	recorder := global.recorder
	global.RUnlock()
	if recorder != nil {
		_ = recorder.Write(ctx, name, fields)
	}
}
