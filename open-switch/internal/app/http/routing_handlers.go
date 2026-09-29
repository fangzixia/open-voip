package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"open-switch/internal/errs"
)

func (d SwitchRouterDeps) handleRoutingSession(w http.ResponseWriter, r *http.Request) {
	if d.Routing == nil {
		writeErr(w, errs.Internal("路由会话查询未配置"))
		return
	}
	view, err := d.Routing.Get(r.Context(), chi.URLParam(r, "callId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
