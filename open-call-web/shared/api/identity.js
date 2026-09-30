// 用户、角色、权限与外部身份映射。
import { apiFetch } from "./client.js";

export function listUsers() {
  return apiFetch("/api/v1/users?page=1&page_size=100");
}

export function createUser(body) {
  return apiFetch("/api/v1/users", { method: "POST", body: JSON.stringify(body) });
}

export function patchUser(id, body) {
  return apiFetch(`/api/v1/users/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function revokeUserSessions(id) {
  return apiFetch(`/api/v1/users/${encodeURIComponent(id)}/revoke-sessions`, { method: "POST" });
}

export function listRoles() {
  return apiFetch("/api/v1/roles");
}

export function listPermissions() {
  return apiFetch("/api/v1/permissions");
}

export function saveRole(id, name, permissions) {
  return apiFetch(`/api/v1/roles/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify({ name, permissions }),
  });
}

export function deleteRole(id) {
  return apiFetch(`/api/v1/roles/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export function setUserRoles(id, roles) {
  return apiFetch(`/api/v1/users/${encodeURIComponent(id)}/roles`, {
    method: "PUT",
    body: JSON.stringify({ roles }),
  });
}

export function listGroupMappings() {
  return apiFetch("/api/v1/identity/group-mappings");
}

export function saveGroupMapping(group, roles) {
  return apiFetch("/api/v1/identity/group-mappings", {
    method: "PUT",
    body: JSON.stringify({ group, roles }),
  });
}

export function listIdentities(id) {
  return apiFetch(`/api/v1/users/${encodeURIComponent(id)}/identities`);
}

export function bindIdentity(id, issuer, subject) {
  return apiFetch(`/api/v1/users/${encodeURIComponent(id)}/identities`, {
    method: "POST",
    body: JSON.stringify({ issuer, subject }),
  });
}

export function unbindIdentity(id, issuer, subject) {
  return apiFetch(
    `/api/v1/users/${encodeURIComponent(id)}/identities?issuer=${encodeURIComponent(issuer)}&subject=${encodeURIComponent(subject)}`,
    { method: "DELETE" },
  );
}
