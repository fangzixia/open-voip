package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"open-call/internal/app/http/middleware"
	"open-call/internal/errs"
	"open-call/internal/layers/biz/auth"
)

func (d RouterDeps) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	pair, err := d.Auth.Login(r.Context(), body.Username, body.Password, auth.SessionMeta{
		UserAgent: r.UserAgent(), RemoteIP: clientIP(r),
	})
	if err != nil {
		d.writeAudit(r.Context(), "", "login", body.Username, map[string]string{"outcome": "failure", "remote_ip": clientIP(r)})
		writeErr(w, err)
		return
	}
	d.writeAudit(r.Context(), "", "login", body.Username, map[string]string{"outcome": "success", "remote_ip": clientIP(r)})
	writeJSON(w, http.StatusOK, pair)
}

func (d RouterDeps) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	pair, err := d.Auth.Refresh(r.Context(), body.RefreshToken)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func (d RouterDeps) handleLogout(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeErr(w, errs.Unauthorized("未认证或令牌失效"))
		return
	}
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = decodeJSON(r, &body)
	if err := d.Auth.Logout(r.Context(), p.JTI, p.ExpiresAt, p.SessionID, body.RefreshToken); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleChangePassword 修改本人密码，成功后全部现有会话失效。
func (d RouterDeps) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFromContext(r.Context())
	if !ok || p.UserID == "" {
		writeErr(w, errs.Unauthorized("未认证或令牌失效"))
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.Auth.ChangePassword(r.Context(), p.UserID, body.CurrentPassword, body.NewPassword); err != nil {
		writeErr(w, err)
		return
	}
	d.writeAudit(r.Context(), p.UserID, "password_change", p.UserID, nil)
	w.WriteHeader(http.StatusNoContent)
}

// handleSessionList 列出本人登录会话。
func (d RouterDeps) handleSessionList(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFromContext(r.Context())
	if !ok || p.UserID == "" {
		writeErr(w, errs.Unauthorized("未认证或令牌失效"))
		return
	}
	items, err := d.Auth.ListSessions(r.Context(), p.UserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "current_session_id": p.SessionID})
}

// handleSessionRevoke 撤销本人指定登录会话。
func (d RouterDeps) handleSessionRevoke(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFromContext(r.Context())
	if !ok || p.UserID == "" {
		writeErr(w, errs.Unauthorized("未认证或令牌失效"))
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionId"))
	if err := d.Auth.RevokeSession(r.Context(), p.UserID, sessionID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// clientIP 返回已由 RealIP 中间件校正的来源地址。
func clientIP(r *http.Request) string {
	value := r.RemoteAddr
	if i := strings.LastIndex(value, ":"); i > -1 {
		value = value[:i]
	}
	return strings.Trim(value, "[]")
}
