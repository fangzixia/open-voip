// 本文件负责实时与历史报表接口。
package http

import "net/http"

func (d RouterDeps) handleLiveReport(w http.ResponseWriter, r *http.Request) {
	out, err := d.Reports.Live(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleHistoricalReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out, err := d.Reports.Historical(r.Context(), q.Get("from"), q.Get("to"), q.Get("queue_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleAgentUtil(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out, err := d.Reports.AgentUtilization(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
