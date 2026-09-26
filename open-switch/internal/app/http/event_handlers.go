package http

import (
	"net/http"
	"open-switch/internal/errs"
	"open-switch/internal/store"
	"strconv"
)

func (d SwitchRouterDeps) handleEvents(w http.ResponseWriter, r *http.Request) {
	after := int64(0)
	if raw := r.URL.Query().Get("after_id"); raw != "" {
		var err error
		after, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || after < 0 {
			writeErr(w, errs.InvalidRequest("after_id 无效"))
			return
		}
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := d.Events.List(r.Context(), after, r.URL.Query().Get("call_id"), limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	if rows == nil {
		rows = []store.CallEventRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}
