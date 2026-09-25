package observability

import (
	"context"
	"encoding/json"
	"log/slog"
)

// RedactingHandler applies last-line credential protection to every log sink.
type RedactingHandler struct{ next slog.Handler }

func NewRedactingHandler(next slog.Handler) slog.Handler { return &RedactingHandler{next: next} }
func (h *RedactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}
func (h *RedactingHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return &RedactingHandler{next: h.next.WithAttrs(redactAttrs(a))}
}
func (h *RedactingHandler) WithGroup(n string) slog.Handler {
	return &RedactingHandler{next: h.next.WithGroup(n)}
}
func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	c := slog.NewRecord(r.Time, r.Level, Redact(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool { c.AddAttrs(redactAttr(a)); return true })
	return h.next.Handle(ctx, c)
}

func redactAttrs(in []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, len(in))
	for i, a := range in {
		out[i] = redactAttr(a)
	}
	return out
}

func redactAttr(a slog.Attr) slog.Attr {
	if SensitiveKey(a.Key) {
		return slog.String(a.Key, "[REDACTED]")
	}
	if a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, Redact(a.Value.String()))
	}
	if a.Value.Kind() == slog.KindAny {
		if err, ok := a.Value.Any().(error); ok {
			return slog.String(a.Key, Redact(err.Error()))
		}
		if raw, err := json.Marshal(a.Value.Any()); err == nil {
			return slog.String(a.Key, Redact(string(raw)))
		}
	}
	if a.Value.Kind() == slog.KindGroup {
		return slog.Group(a.Key, anyAttrs(redactAttrs(a.Value.Group()))...)
	}
	return a
}

func anyAttrs(attrs []slog.Attr) []any {
	out := make([]any, len(attrs))
	for i := range attrs {
		out[i] = attrs[i]
	}
	return out
}

type MultiHandler struct{ handlers []slog.Handler }

func NewMultiHandler(handlers ...slog.Handler) slog.Handler { return &MultiHandler{handlers: handlers} }
func (h *MultiHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for _, x := range h.handlers {
		if x.Enabled(ctx, l) {
			return true
		}
	}
	return false
}
func (h *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	var first error
	for _, x := range h.handlers {
		if x.Enabled(ctx, r.Level) {
			if err := x.Handle(ctx, r.Clone()); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}
func (h *MultiHandler) WithAttrs(a []slog.Attr) slog.Handler {
	out := make([]slog.Handler, len(h.handlers))
	for i, x := range h.handlers {
		out[i] = x.WithAttrs(a)
	}
	return &MultiHandler{handlers: out}
}
func (h *MultiHandler) WithGroup(n string) slog.Handler {
	out := make([]slog.Handler, len(h.handlers))
	for i, x := range h.handlers {
		out[i] = x.WithGroup(n)
	}
	return &MultiHandler{handlers: out}
}

// TraceOnlyHandler writes only records carrying the event field.
type TraceOnlyHandler struct{ next slog.Handler }

func NewTraceOnlyHandler(next slog.Handler) slog.Handler { return &TraceOnlyHandler{next: next} }
func (h *TraceOnlyHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}
func (h *TraceOnlyHandler) Handle(ctx context.Context, r slog.Record) error {
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == EventKey {
			found = true
		}
		return !found
	})
	if !found {
		return nil
	}
	return h.next.Handle(ctx, r)
}
func (h *TraceOnlyHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return &TraceOnlyHandler{next: h.next.WithAttrs(a)}
}
func (h *TraceOnlyHandler) WithGroup(n string) slog.Handler {
	return &TraceOnlyHandler{next: h.next.WithGroup(n)}
}
