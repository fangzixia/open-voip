package http

import (
	"net/http"
	"open-call/internal/errs"
)

func (d RouterDeps) handleConfigPublish(w http.ResponseWriter, r *http.Request) {
	if d.ConfigPublisher == nil {
		writeErr(w, errs.Internal("配置发布服务未装配"))
		return
	}
	view, err := d.ConfigPublisher.Publish(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
