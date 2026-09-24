package http

import (
	"context"
	"github.com/go-chi/chi/v5"
	"net/http"
	"open-switch/internal/ports"
)

// RuntimeReader 仅供可信业务服务读取运行时，不允许通过浏览器 BFF 代理。
type RuntimeReader interface {
	ListCalls(context.Context) ([]ports.CallView, error)
}

func (d SwitchRouterDeps) handleInternalCall(w http.ResponseWriter, r *http.Request) {
	v, err := d.Signaling.GetCall(r.Context(), chi.URLParam(r, "callId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (d SwitchRouterDeps) handleInternalCalls(w http.ResponseWriter, r *http.Request) {
	v, err := d.Runtime.ListCalls(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
