package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"open-voip/internal/errs"
	"open-voip/internal/layers/biz/queue"
	"open-voip/internal/ports/dto"
)

func (d RouterDeps) handleQueueList(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	out, err := d.Queues.List(r.Context(), page, size)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleQueueCreate(w http.ResponseWriter, r *http.Request) {
	var in queue.CreateInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Queues.Create(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleQueueGet(w http.ResponseWriter, r *http.Request) {
	out, err := d.Queues.Get(r.Context(), chi.URLParam(r, "queueId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleQueuePatch(w http.ResponseWriter, r *http.Request) {
	var in queue.CreateInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Queues.Update(r.Context(), chi.URLParam(r, "queueId"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleQueueDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Queues.Delete(r.Context(), chi.URLParam(r, "queueId")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d RouterDeps) handleQueueBind(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentIDs []string `json:"agent_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.Queues.BindAgents(r.Context(), chi.URLParam(r, "queueId"), body.AgentIDs); err != nil {
		writeErr(w, err)
		return
	}
	ids, _ := d.Queues.AgentIDs(r.Context(), chi.URLParam(r, "queueId"))
	writeJSON(w, http.StatusOK, map[string]any{"agent_ids": ids})
}

func (d RouterDeps) handleGuestQueues(w http.ResponseWriter, r *http.Request) {
	items, err := d.Queues.ListPublic(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	type item struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		VideoEnabled     bool   `json:"video_enabled"`
		PriorityEnabled  bool   `json:"priority_enabled"`
	}
	out := make([]item, 0, len(items))
	for _, q := range items {
		out = append(out, item{ID: q.ID, Name: q.Name, VideoEnabled: q.VideoEnabled, PriorityEnabled: q.PriorityEnabled})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (d RouterDeps) handleGuestJoin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		QueueID     string `json:"queue_id"`
		SessionType string `json:"session_type"`
		Token       string `json:"token"`
		Priority    int    `json:"priority"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Token != "" {
		out, err := d.Guests.JoinByToken(r.Context(), body.Token, dto.SessionType(body.SessionType))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
		return
	}
	out, err := d.Guests.Join(r.Context(), body.QueueID, dto.SessionType(body.SessionType), body.Priority)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleGuestSessions(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok {
		writeErr(w, errs.Unauthorized("未认证或令牌失效"))
		return
	}
	if p.Role != "admin" && p.Role != "agent" && p.Role != "supervisor" {
		writeErr(w, errs.Forbidden("无权限"))
		return
	}
	var body struct {
		QueueID      string `json:"queue_id"`
		TTLSec       int    `json:"ttl_sec"`
		AllowedMedia string `json:"allowed_media"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Guests.CreateSession(r.Context(), body.QueueID, body.TTLSec, body.AllowedMedia)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleCDRList(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	q := r.URL.Query()
	out, err := d.CDR.List(r.Context(), page, size, q.Get("from"), q.Get("to"), q.Get("queue_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
