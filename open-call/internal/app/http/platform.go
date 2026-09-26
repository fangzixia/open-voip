// 本文件负责平台回调接口。
package http

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"gorm.io/gorm"

	"open-call/internal/layers/biz/webhook"
	"open-call/internal/ports"
)

type StatusProvider struct {
	DB      *gorm.DB
	Runtime interface {
		ListCalls(context.Context) ([]ports.CallView, error)
	}
	WSConnections func() int
	Webhooks      interface {
		Stats(context.Context) (webhook.Stats, error)
	}
	Audit interface{ WriteFailures() uint64 }
}

type statusResponse struct {
	Ready              bool          `json:"ready"`
	DBOK               bool          `json:"db_ok"`
	SwitchOK           bool          `json:"switch_ok"`
	ActiveCalls        int           `json:"active_calls"`
	WSConnections      int           `json:"ws_connections"`
	Webhook            webhook.Stats `json:"webhook"`
	AuditWriteFailures uint64        `json:"audit_write_failures"`
	Goroutines         int           `json:"goroutines"`
	HeapAllocBytes     uint64        `json:"heap_alloc_bytes"`
	CheckedAt          time.Time     `json:"checked_at"`
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s StatusProvider) snapshot(ctx context.Context) statusResponse {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	out := statusResponse{Goroutines: runtime.NumGoroutine(), HeapAllocBytes: mem.HeapAlloc, CheckedAt: time.Now().UTC()}
	if s.DB != nil {
		if sqlDB, err := s.DB.DB(); err == nil {
			c, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			out.DBOK = sqlDB.PingContext(c) == nil
		}
	}
	if s.Runtime != nil {
		c, cancel := context.WithTimeout(ctx, 3*time.Second)
		calls, err := s.Runtime.ListCalls(c)
		cancel()
		if err == nil {
			out.SwitchOK = true
			for _, call := range calls {
				if call.State != "ended" {
					out.ActiveCalls++
				}
			}
		}
	}
	if s.WSConnections != nil {
		out.WSConnections = s.WSConnections()
	}
	if s.Webhooks != nil {
		if stats, err := s.Webhooks.Stats(ctx); err == nil {
			out.Webhook = stats
		}
	}
	if s.Audit != nil {
		out.AuditWriteFailures = s.Audit.WriteFailures()
	}
	out.Ready = out.DBOK && out.SwitchOK
	return out
}

func (s StatusProvider) handleReady(w http.ResponseWriter, r *http.Request) {
	out := s.snapshot(r.Context())
	if !out.Ready {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ready": false, "db_ok": out.DBOK, "switch_ok": out.SwitchOK})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ready": true})
}
func (s StatusProvider) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.snapshot(r.Context()))
}
