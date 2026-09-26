// 本文件负责OIDC 登录与凭据兑换接口。
package http

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"open-call/internal/errs"
	"open-call/internal/layers/biz/auth"
)

func (d RouterDeps) handleAuthOptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"oidc_enabled": d.OIDC != nil, "local_login_enabled": true, "local_login_emergency_only": d.OIDC != nil})
}

func (d RouterDeps) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	target, err := d.OIDC.Start(r.Context(), r.URL.Query().Get("return_to"))
	if err != nil {
		writeErr(w, err)
		return
	}
	u, err := url.Parse(target)
	if err != nil {
		writeErr(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "open_voip_oidc_state", Value: u.Query().Get("state"), Path: "/api/v1/auth/oidc/callback", MaxAge: 300, HttpOnly: true, Secure: oidcCookieSecure(d.Config.OIDC.RedirectURL), SameSite: http.SameSiteLaxMode})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

func (d RouterDeps) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("open_voip_oidc_state")
	state := r.URL.Query().Get("state")
	if err != nil || cookie.Value == "" || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		writeErr(w, errs.Unauthorized("单点登录状态无效"))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "open_voip_oidc_state", Path: "/api/v1/auth/oidc/callback", MaxAge: -1, HttpOnly: true, Secure: oidcCookieSecure(d.Config.OIDC.RedirectURL), SameSite: http.SameSiteLaxMode})
	if r.URL.Query().Get("error") != "" {
		writeErr(w, errs.Unauthorized("身份平台拒绝登录"))
		return
	}
	ticket, path, err := d.OIDC.Callback(r.Context(), state, r.URL.Query().Get("code"))
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, d.OIDC.Redirect(ticket, path), http.StatusFound)
}

func oidcCookieSecure(redirectURL string) bool {
	u, err := url.Parse(redirectURL)
	return err == nil && u.Scheme == "https"
}

func (d RouterDeps) handleOIDCExchange(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Ticket string `json:"ticket"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	u, refresh, subject, err := d.OIDC.Exchange(r.Context(), in.Ticket)
	if err != nil {
		writeErr(w, err)
		return
	}
	pair, err := d.Auth.IssueOIDC(r.Context(), u, refresh, subject, auth.SessionMeta{UserAgent: r.UserAgent(), RemoteIP: clientIP(r)})
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, pair)
}
