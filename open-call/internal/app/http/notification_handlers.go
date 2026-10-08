package http

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"open-call/internal/errs"
)

func (d RouterDeps) handleNotificationSubmit(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok || p.IsGuest() || p.AgentID == "" {
		writeErr(w, errs.Forbidden("仅坐席可发起通知"))
		return
	}
	var body struct {
		Destination string `json:"destination"`
		AssetID     string `json:"prompt_asset_id"`
		TrunkID     string `json:"trunk_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	t, err := d.Notifications.Submit(r.Context(), p.UserID, r.Header.Get("Idempotency-Key"), body.Destination, body.TrunkID, body.AssetID)
	if err != nil {
		writeErr(w, err)
		return
	}
	d.writeAudit(r.Context(), p.UserID, "notification.submit", t.ID, nil)
	writeJSON(w, http.StatusAccepted, t)
}
func (d RouterDeps) handleNotificationGet(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok || p.IsGuest() {
		writeErr(w, errs.Forbidden("无权读取通知"))
		return
	}
	t, err := d.Notifications.Get(r.Context(), p.UserID, chi.URLParam(r, "taskId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}
func (d RouterDeps) handleNotificationCancel(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok || p.IsGuest() {
		writeErr(w, errs.Forbidden("无权取消通知"))
		return
	}
	if err := d.Notifications.Cancel(r.Context(), p.UserID, chi.URLParam(r, "taskId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"state": "stopping"})
}
