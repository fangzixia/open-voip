// 鉴权与会话。
import { apiFetch } from "./client.js";

export function login(loginName, password) {
  return apiFetch("/api/v1/auth/login", {
    method: "POST",
    auth: false,
    body: JSON.stringify({ login_name: loginName, password }),
  });
}

export function authOptions() {
  return apiFetch("/api/v1/auth/options", { auth: false });
}

export function authMe() {
  return apiFetch("/api/v1/auth/me");
}

export function changePassword(currentPassword, newPassword) {
  return apiFetch("/api/v1/auth/change-password", {
    method: "POST",
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  });
}

export function exchangeSSOTicket(ticket) {
  return apiFetch("/api/v1/auth/oidc/exchange", {
    method: "POST",
    auth: false,
    body: JSON.stringify({ ticket }),
  });
}

export function startSSO() {
  const base = new URL(
    import.meta.env?.VITE_API_BASE || window.__OPEN_VOIP__?.apiBase || window.location.origin,
    window.location.origin,
  );
  const url = new URL("/api/v1/auth/oidc/start", base);
  window.location.assign(url.href);
}

export function popSSOTicket() {
  const hash = window.location.hash.replace(/^#/, "");
  const ticket = new URLSearchParams(hash).get("sso_ticket");
  if (ticket) history.replaceState(null, "", window.location.pathname + window.location.search);
  return ticket;
}

export function logout() {
  return apiFetch("/api/v1/auth/logout", { method: "POST" });
}
