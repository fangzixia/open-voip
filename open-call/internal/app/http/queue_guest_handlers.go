package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"open-call/internal/errs"
	"open-call/internal/layers/biz/cdr"
	"open-call/internal/layers/biz/queue"
	"open-call/internal/ports/dto"
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
	var in queue.UpdateInput
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
	writeJSON(w, http.StatusOK, nil)
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
	if !d.Config.Public.AllowDirectJoin {
		writeErr(w, errs.Forbidden("公开队列列表未启用"))
		return
	}
	items, err := d.Queues.ListPublic(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	type item struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		VideoEnabled    bool   `json:"video_enabled"`
		PriorityEnabled bool   `json:"priority_enabled"`
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
		UserID      string `json:"user_id"`
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
	if !d.Config.Public.AllowDirectJoin {
		writeErr(w, errs.Forbidden("生产模式仅允许使用已签发的访客令牌"))
		return
	}
	out, err := d.Guests.Join(r.Context(), body.QueueID, dto.SessionType(body.SessionType), body.Priority, body.UserID)
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
		Priority     int    `json:"priority"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Guests.CreateSession(r.Context(), body.QueueID, body.TTLSec, body.AllowedMedia, body.Priority)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleCDRList(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	q := r.URL.Query()
	out, err := d.CDR.ListFiltered(r.Context(), page, size, cdr.Filter{
		From: q.Get("from"), To: q.Get("to"), QueueID: q.Get("queue_id"),
		AgentID: q.Get("agent_id"), Direction: q.Get("direction"), Result: q.Get("result"),
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
