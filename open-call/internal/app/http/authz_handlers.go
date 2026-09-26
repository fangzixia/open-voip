// 本文件负责用户、角色和权限管理接口。
package http

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"open-call/internal/app/http/middleware"
	"open-call/internal/errs"
)

func (d RouterDeps) handleMe(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeErr(w, errs.Unauthorized("未认证"))
		return
	}
	roles, err := d.Authorization.UserRoles(r.Context(), p.UserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": p.UserID, "agent_id": p.AgentID, "roles": roles, "permissions": p.Permissions})
}

func (d RouterDeps) handlePermissionList(w http.ResponseWriter, r *http.Request) {
	out, err := d.Authorization.ListPermissions(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
func (d RouterDeps) handleRoleList(w http.ResponseWriter, r *http.Request) {
	out, err := d.Authorization.ListRoles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
func (d RouterDeps) handleRoleSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.Authorization.SaveRole(r.Context(), chi.URLParam(r, "roleId"), in.Name, in.Permissions); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
func (d RouterDeps) handleRoleDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Authorization.DeleteRole(r.Context(), chi.URLParam(r, "roleId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
func (d RouterDeps) handleUserRoles(w http.ResponseWriter, r *http.Request) {
	out, err := d.Authorization.UserRoles(r.Context(), chi.URLParam(r, "userId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": out})
}
func (d RouterDeps) handleUserRolesPut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Roles []string `json:"roles"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if d.Config.OIDC.Enabled && d.isEmergencyAdmin(r, chi.URLParam(r, "userId")) {
		found := false
		for _, id := range in.Roles {
			if id == "admin" {
				found = true
			}
		}
		if !found {
			writeErr(w, errs.Forbidden("应急管理员必须保留 admin 角色"))
			return
		}
	}
	if err := d.Authorization.SetUserRoles(r.Context(), chi.URLParam(r, "userId"), in.Roles); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
func (d RouterDeps) handleGroupMappings(w http.ResponseWriter, r *http.Request) {
	out, err := d.Authorization.ListMappings(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
func (d RouterDeps) handleGroupMappingSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Group string   `json:"group"`
		Roles []string `json:"roles"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	group := chi.URLParam(r, "group")
	if group == "" {
		group = in.Group
	}
	if err := d.Authorization.SetMapping(r.Context(), group, in.Roles); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
func (d RouterDeps) handleIdentities(w http.ResponseWriter, r *http.Request) {
	out, err := d.Authorization.ListIdentities(r.Context(), chi.URLParam(r, "userId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
func (d RouterDeps) handleIdentityBind(w http.ResponseWriter, r *http.Request) {
	if d.Config.OIDC.Enabled && d.isEmergencyAdmin(r, chi.URLParam(r, "userId")) {
		writeErr(w, errs.Forbidden("应急管理员不可绑定外部身份"))
		return
	}
	var in struct {
		Issuer  string `json:"issuer"`
		Subject string `json:"subject"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if d.Config.OIDC.Enabled && in.Issuer != d.Config.OIDC.Issuer {
		writeErr(w, errs.InvalidRequest("issuer 必须与已配置的身份平台一致"))
		return
	}
	if err := d.Authorization.BindIdentity(r.Context(), chi.URLParam(r, "userId"), in.Issuer, in.Subject); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
func (d RouterDeps) handleIdentityDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Authorization.DeleteIdentity(r.Context(), chi.URLParam(r, "userId"), r.URL.Query().Get("issuer"), r.URL.Query().Get("subject")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
