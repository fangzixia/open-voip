// 本文件负责通话小结与质检标记接口。
package http

import (
	"net/http"
	"open-call/internal/errs"

	"github.com/go-chi/chi/v5"
)

func (d RouterDeps) handleWrapUp(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok || p.AgentID == "" {
		writeErr(w, errs.Forbidden("仅坐席可提交小结"))
		return
	}
	var body struct {
		Notes           string   `json:"notes"`
		Text            string   `json:"text"`
		DispositionCode string   `json:"disposition_code"`
		Tags            []string `json:"tags"`
		Complete        bool     `json:"complete"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	notes := body.Notes
	if notes == "" {
		notes = body.Text
	}
	if d.Recordings == nil {
		writeErr(w, errs.NotImplemented("小结"))
		return
	}
	out, err := d.Recordings.SaveWrapUp(r.Context(), chi.URLParam(r, "callId"), p.AgentID, notes, body.DispositionCode, body.Tags, body.Complete)
	if err != nil {
		writeErr(w, err)
		return
	}
	if body.Complete && d.AgentRuntime != nil {
		_, _ = d.AgentRuntime.SetPresence(r.Context(), p.AgentID, "idle", "wrap-up")
	}
	d.writeAudit(r.Context(), p.UserID, "wrap_up", chi.URLParam(r, "callId"), nil)
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleWrapUpList(w http.ResponseWriter, r *http.Request) {
	if d.Recordings == nil {
		writeErr(w, errs.NotImplemented("小结"))
		return
	}
	out, err := d.Recordings.ListWrapUps(r.Context(), r.URL.Query().Get("call_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (d RouterDeps) handleQAList(w http.ResponseWriter, r *http.Request) {
	out, err := d.Recordings.ListQAMarks(r.Context(), chi.URLParam(r, "callId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (d RouterDeps) handleQACreate(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok {
		writeErr(w, errs.Unauthorized("未认证或令牌失效"))
		return
	}
	var body struct {
		OffsetSec int    `json:"offset_sec"`
		Label     string `json:"label"`
		Score     *int   `json:"score"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Recordings.AddQAMark(r.Context(), chi.URLParam(r, "callId"), p.UserID, body.OffsetSec, body.Label, body.Score)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleQAUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OffsetSec int    `json:"offset_sec"`
		Label     string `json:"label"`
		Score     *int   `json:"score"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Recordings.UpdateQAMark(r.Context(), chi.URLParam(r, "markId"), chi.URLParam(r, "callId"), body.OffsetSec, body.Label, body.Score)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleQADelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Recordings.DeleteQAMark(r.Context(), chi.URLParam(r, "markId"), chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
