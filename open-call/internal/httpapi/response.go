// Package httpapi defines the common CC/Switch JSON response contract.
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"open-call/internal/datetime"
	"open-call/internal/errs"
	"open-call/internal/observability"
)

type Response struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data"`
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
}

func Write(w http.ResponseWriter, status int, data any) {
	if status >= 400 {
		kind := strings.ToLower(strings.ReplaceAll(http.StatusText(status), " ", "_"))
		write(w, status, Response{Code: strings.ToUpper(kind), Message: "服务暂不可用", Data: data, Error: kind})
		return
	}
	if status == http.StatusNoContent {
		status = http.StatusOK
	}
	write(w, status, Response{Code: "OK", Message: "成功", Data: data})
}

func Error(w http.ResponseWriter, err error) {
	api := errs.AsAPIError(err)
	code := api.Code
	if code == "" {
		code = strings.ToUpper(api.Kind)
	}
	write(w, api.HTTP, Response{Code: code, Message: api.Message, Error: api.Kind})
}

func Failure(w http.ResponseWriter, status int, kind, message string) {
	Error(w, &errs.APIError{HTTP: status, Kind: kind, Message: message})
}

func write(w http.ResponseWriter, status int, body Response) {
	body.RequestID = w.Header().Get("X-Request-ID")
	raw, err := datetime.Marshal(body)
	if err != nil {
		slog.Error("encode API response", "error", err)
		status = http.StatusInternalServerError
		raw, _ = json.Marshal(Response{Code: "INTERNAL_ERROR", Message: "服务器内部错误", Error: "internal_error", RequestID: body.RequestID})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(raw, '\n'))
}

// RequestID keeps the same ID across the browser, CC and Switch.
func RequestID(next http.Handler) http.Handler {
	instrumented := chimw.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := chimw.GetReqID(r.Context())
		traceID := observability.NormalizeID(r.Header.Get("X-Trace-ID"))
		if traceID == "" {
			traceID = observability.NewID()
		}
		r.Header.Set("X-Request-ID", id)
		r.Header.Set("X-Trace-ID", traceID)
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Trace-ID", traceID)
		ctx := observability.With(r.Context(), observability.Context{
			TraceID: traceID, RequestID: id,
			CallID:          observability.NormalizeID(r.Header.Get("X-Call-ID")),
			LegID:           observability.NormalizeID(r.Header.Get("X-Leg-ID")),
			ClientSessionID: observability.NormalizeID(r.Header.Get("X-Client-Session-ID")),
		})
		started := time.Now()
		observability.Emit(ctx, "http.request.started", map[string]any{"method": r.Method, "path": r.URL.Path})
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			observability.Emit(ctx, "http.request.completed", map[string]any{
				"method": r.Method, "path": r.URL.Path, "status": status,
				"duration_ms": time.Since(started).Milliseconds(),
			})
		}()
		next.ServeHTTP(ww, r.WithContext(ctx))
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// BFF 外层与业务 Router 都会挂载该中间件；已有上下文时只复用，
		// 避免同一请求写入两组 started/completed 事件。
		if fields := observability.From(r.Context()); fields.RequestID != "" {
			w.Header().Set("X-Request-ID", fields.RequestID)
			w.Header().Set("X-Trace-ID", fields.TraceID)
			next.ServeHTTP(w, r)
			return
		}
		instrumented.ServeHTTP(w, r)
	})
}
func ID(ctx context.Context) string      { return chimw.GetReqID(ctx) }
func TraceID(ctx context.Context) string { return observability.From(ctx).TraceID }

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			if value := recover(); value != nil {
				if value == http.ErrAbortHandler {
					panic(value)
				}
				slog.Error("HTTP panic", "request_id", w.Header().Get("X-Request-ID"), "panic", value)
				if ww.Status() == 0 {
					Failure(ww, 500, "internal_error", "服务器内部错误")
				}
			}
		}()
		next.ServeHTTP(ww, r)
	})
}
func NotFound(w http.ResponseWriter, r *http.Request) {
	Failure(w, 404, "not_found", "接口不存在")
}
func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	Failure(w, 405, "method_not_allowed", "请求方法不允许")
}

// Decode accepts the unified response and legacy peers during a rolling upgrade.
func Decode(res *http.Response, out any) error {
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read API response: %w", err)
	}
	var env struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Error   string          `json:"error"`
		Data    json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(raw, &env)
	if res.StatusCode < 200 || res.StatusCode >= 300 || (env.Data != nil && env.Code != "" && env.Code != "OK") {
		status := res.StatusCode
		if status < 400 {
			status = http.StatusBadGateway
		}
		if env.Error == "" {
			env.Error = "upstream_error"
		}
		if env.Message == "" {
			env.Message = "上游服务请求失败"
		}
		return &errs.APIError{Kind: env.Error, Message: env.Message, Code: env.Code, HTTP: status}
	}
	if env.Code == "OK" && env.Data != nil {
		raw = env.Data
	}
	if out != nil && len(raw) > 0 && string(raw) != "null" {
		if err := datetime.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode API response: %w", err)
		}
	}
	return nil
}
