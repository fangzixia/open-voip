// 本文件负责审计日志查询接口。
package http

import "net/http"

func (d RouterDeps) handleAuditList(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	q := r.URL.Query()
	out, err := d.Audit.List(r.Context(), page, size, q.Get("user_id"), q.Get("action"),
		q.Get("resource"), q.Get("outcome"), q.Get("from"), q.Get("to"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
