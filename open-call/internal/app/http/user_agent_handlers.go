package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"open-call/internal/errs"
	"open-call/internal/layers/biz/user"
)

func (d RouterDeps) handleUserList(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	out, err := d.Users.List(r.Context(), page, size)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	var in user.CreateInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Users.Create(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleUserGet(w http.ResponseWriter, r *http.Request) {
	out, err := d.Users.Get(r.Context(), chi.URLParam(r, "userId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleUserPatch(w http.ResponseWriter, r *http.Request) {
	var in user.UpdateInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Users.Update(r.Context(), chi.URLParam(r, "userId"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Users.Delete(r.Context(), chi.URLParam(r, "userId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d RouterDeps) handleUserResetPassword(w http.ResponseWriter, r *http.Request) {
	plain, err := d.Users.ResetPassword(r.Context(), chi.URLParam(r, "userId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"password": plain})
}

// handleUserRevokeSessions 由管理员强制撤销指定账号全部登录会话。
func (d RouterDeps) handleUserRevokeSessions(w http.ResponseWriter, r *http.Request) {
	if err := d.Auth.RevokeAllSessions(r.Context(), chi.URLParam(r, "userId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d RouterDeps) handleAgentMe(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok {
		writeErr(w, errs.Unauthorized("未认证或令牌失效"))
		return
	}
	id := p.AgentID
	if id == "" {
		writeErr(w, errs.NotFound("当前账号没有坐席资料"))
		return
	}
	out, err := d.Agents.Me(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleCheckIn(w http.ResponseWriter, r *http.Request) {
	if err := d.ensureSelfAgent(r, chi.URLParam(r, "agentId")); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		QueueIDs []string `json:"queue_ids"`
	}
	_ = decodeJSON(r, &body)
	out, err := d.Agents.CheckIn(r.Context(), chi.URLParam(r, "agentId"), body.QueueIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleCheckOut(w http.ResponseWriter, r *http.Request) {
	if err := d.ensureSelfAgent(r, chi.URLParam(r, "agentId")); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.Agents.CheckOut(r.Context(), chi.URLParam(r, "agentId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d RouterDeps) handleAgentState(w http.ResponseWriter, r *http.Request) {
	if err := d.ensureSelfAgent(r, chi.URLParam(r, "agentId")); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		State      string `json:"state"`
		BusyReason string `json:"busy_reason"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Agents.UpdateState(r.Context(), chi.URLParam(r, "agentId"), body.State, body.BusyReason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) ensureSelfAgent(r *http.Request, agentID string) error {
	p, ok := principal(r)
	if !ok {
		return errs.Unauthorized("未认证或令牌失效")
	}
	if p.Role == "admin" {
		return nil
	}
	if p.AgentID == "" || p.AgentID != agentID {
		return errs.Forbidden("只能操作自己的坐席")
	}
	return nil
}
