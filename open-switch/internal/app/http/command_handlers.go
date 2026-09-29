package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"open-switch/internal/errs"
)

func (d SwitchRouterDeps) handleCommandGet(w http.ResponseWriter, r *http.Request) {
	if d.Commands == nil {
		writeErr(w, errNotConfigured("命令存储"))
		return
	}
	view, err := d.Commands.Get(r.Context(), chi.URLParam(r, "commandId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func errNotConfigured(feature string) error {
	return errs.Internal(feature + " 未配置")
}
