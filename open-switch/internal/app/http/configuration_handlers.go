package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
)

func (d SwitchRouterDeps) handleConfigStore(w http.ResponseWriter, r *http.Request) {
	var bundle ports.ConfigBundle
	if err := decodeJSON(r, &bundle); err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.Admin.StoreConfig(r.Context(), bundle)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func configVersion(r *http.Request) (int64, error) {
	version, err := strconv.ParseInt(chi.URLParam(r, "version"), 10, 64)
	if err != nil || version < 1 {
		return 0, errs.InvalidRequest("version 无效")
	}
	return version, nil
}

func (d SwitchRouterDeps) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	version, err := configVersion(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.Admin.GetConfigVersion(r.Context(), version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (d SwitchRouterDeps) handleConfigActivate(w http.ResponseWriter, r *http.Request) {
	version, err := configVersion(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.Admin.ActivateConfig(r.Context(), version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (d SwitchRouterDeps) handleAgentCheckIn(w http.ResponseWriter, r *http.Request) {
	var body struct {
		QueueIDs []string `json:"queue_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.Admin.CheckIn(r.Context(), chi.URLParam(r, "agentId"), body.QueueIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (d SwitchRouterDeps) handleAgentCheckOut(w http.ResponseWriter, r *http.Request) {
	if err := d.Admin.CheckOut(r.Context(), chi.URLParam(r, "agentId")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d SwitchRouterDeps) handleAgentPresence(w http.ResponseWriter, r *http.Request) {
	var body struct {
		State  string `json:"state"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.Admin.SetPresence(r.Context(), chi.URLParam(r, "agentId"), body.State, body.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (d SwitchRouterDeps) handleAgentSession(w http.ResponseWriter, r *http.Request) {
	view, err := d.Admin.AgentSession(r.Context(), chi.URLParam(r, "agentId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (d SwitchRouterDeps) handleAgentSessionList(w http.ResponseWriter, r *http.Request) {
	items, err := d.Admin.ListAgentSessions(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (d SwitchRouterDeps) handleQueueStatus(w http.ResponseWriter, r *http.Request) {
	view, err := d.Admin.QueueStatus(r.Context(), chi.URLParam(r, "queueId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
