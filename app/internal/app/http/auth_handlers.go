package http

import (
	"net/http"

	"open-voip/internal/app/http/middleware"
	"open-voip/internal/errs"
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
	pair, err := d.Auth.Login(r.Context(), body.Username, body.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	if d.Audit != nil {
		d.Audit.Write(r.Context(), "", "login", body.Username, nil)
	}
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
	if err := d.Auth.Logout(r.Context(), p.JTI, p.ExpiresAt, body.RefreshToken); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
