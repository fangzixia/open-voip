import { request } from "./http-client.js";
import { formatDate, formatDateTime } from "./datetime.js";
const apiFetch = request;

export function fetchStatus() {
  return apiFetch("/api/v1/status", { method: "GET" });
}

export function login(loginName, password) {
  return apiFetch("/api/v1/auth/login", {
    method: "POST",
    auth: false,
    body: JSON.stringify({ login_name: loginName, password }),
  });
}
export function authOptions() { return apiFetch("/api/v1/auth/options", { auth: false }); }
export function authMe() { return apiFetch("/api/v1/auth/me"); }
export function changePassword(currentPassword, newPassword) {
  return apiFetch("/api/v1/auth/change-password", { method: "POST", body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }) });
}
export function exchangeSSOTicket(ticket) { return apiFetch("/api/v1/auth/oidc/exchange", { method: "POST", auth: false, body: JSON.stringify({ ticket }) }); }
export function startSSO() {
  const base = new URL(import.meta.env?.VITE_API_BASE || window.__OPEN_VOIP__?.apiBase || window.location.origin, window.location.origin);
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

export function fetchAgentMe() {
  return apiFetch("/api/v1/agents/me");
}

export function fetchMyCalls() {
  return apiFetch("/api/v1/agents/me/calls");
}

export function checkIn(agentId, queueIds) {
  return apiFetch(`/api/v1/agents/${agentId}/check-in`, {
    method: "POST",
    body: JSON.stringify({ queue_ids: queueIds }),
  });
}

export function checkOut(agentId) {
  return apiFetch(`/api/v1/agents/${agentId}/check-out`, { method: "POST" });
}

export function setAgentState(agentId, state, busyReason = "") {
  return apiFetch(`/api/v1/agents/${agentId}/state`, {
    method: "PUT",
    body: JSON.stringify({ state, busy_reason: busyReason }),
  });
}

export function listQueues(page = 1) {
  return apiFetch(`/api/v1/queues?page=${page}&page_size=100`);
}

export function listGuestQueues() {
  return apiFetch("/api/v1/guest/queues", { auth: false });
}

export function guestJoin(queueId, sessionType, priority = 0, userId = "") {
  return apiFetch("/api/v1/guest/join", {
    method: "POST",
    auth: false,
    body: JSON.stringify({ queue_id: queueId, session_type: sessionType, priority, user_id: userId }),
  });
}

export function getCall(callId) {
  return apiFetch(`/api/v1/calls/${callId}`);
}

export function answerCall(callId) {
  return apiFetch(`/api/v1/calls/${callId}/answer`, { method: "POST" });
}

export function hangupCall(callId, reason = "normal") {
  return apiFetch(`/api/v1/calls/${callId}/hangup`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  });
}

export function postOffer(callId, legId) {
  return apiFetch(`/api/v1/calls/${callId}/legs/${legId}/offer`, {
    method: "POST",
    body: JSON.stringify({ type: "offer" }),
  });
}

export function postAnswer(callId, legId, sdp) {
  return apiFetch(`/api/v1/calls/${callId}/legs/${legId}/answer`, {
    method: "POST",
    body: JSON.stringify({ type: "answer", sdp }),
  });
}

export function postIce(callId, legId, candidate) {
  return apiFetch(`/api/v1/calls/${callId}/legs/${legId}/ice`, {
    method: "POST",
    body: JSON.stringify(candidate),
  });
}

export function postMute(callId, legId, audio, video) {
  return apiFetch(`/api/v1/calls/${callId}/legs/${legId}/mute`, {
    method: "POST",
    body: JSON.stringify({ audio, video }),
  });
}

export function listUsers() {
  return apiFetch("/api/v1/users?page=1&page_size=100");
}

export function createUser(body) {
  return apiFetch("/api/v1/users", { method: "POST", body: JSON.stringify(body) });
}
export function patchUser(id,body) { return apiFetch(`/api/v1/users/${encodeURIComponent(id)}`, { method: "PATCH", body: JSON.stringify(body) }); }
export function revokeUserSessions(id) { return apiFetch(`/api/v1/users/${encodeURIComponent(id)}/revoke-sessions`, { method: "POST" }); }
export function listRoles() { return apiFetch("/api/v1/roles"); }
export function listPermissions() { return apiFetch("/api/v1/permissions"); }
export function saveRole(id,name,permissions) { return apiFetch(`/api/v1/roles/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify({ name, permissions }) }); }
export function deleteRole(id) { return apiFetch(`/api/v1/roles/${encodeURIComponent(id)}`, { method: "DELETE" }); }
export function setUserRoles(id,roles) { return apiFetch(`/api/v1/users/${encodeURIComponent(id)}/roles`, { method: "PUT", body: JSON.stringify({ roles }) }); }
export function listGroupMappings() { return apiFetch("/api/v1/identity/group-mappings"); }
export function saveGroupMapping(group,roles) { return apiFetch("/api/v1/identity/group-mappings", { method: "PUT", body: JSON.stringify({ group, roles }) }); }
export function listIdentities(id) { return apiFetch(`/api/v1/users/${encodeURIComponent(id)}/identities`); }
export function bindIdentity(id,issuer,subject) { return apiFetch(`/api/v1/users/${encodeURIComponent(id)}/identities`, { method: "POST", body: JSON.stringify({ issuer, subject }) }); }
export function unbindIdentity(id,issuer,subject) { return apiFetch(`/api/v1/users/${encodeURIComponent(id)}/identities?issuer=${encodeURIComponent(issuer)}&subject=${encodeURIComponent(subject)}`, { method: "DELETE" }); }

export function createQueue(body) {
  return apiFetch("/api/v1/queues", { method: "POST", body: JSON.stringify(body) });
}

export function bindQueueAgents(queueId, agentIds) {
  return apiFetch(`/api/v1/queues/${queueId}/agents`, {
    method: "PUT",
    body: JSON.stringify({ agent_ids: agentIds }),
  });
}

export function listCdr() {
  return apiFetch(" /api/v1/cdr?page=1&page_size=50".trim());
}

export function outboundCall(destination) {
  return apiFetch("/api/v1/calls/outbound", {
    method: "POST",
    body: JSON.stringify({ destination }),
  });
}

export function holdCall(callId, on) {
  return apiFetch(`/api/v1/calls/${callId}/hold`, {
    method: "POST",
    body: JSON.stringify({ on }),
  });
}

export function transferCall(callId, body) {
  return apiFetch(`/api/v1/calls/${callId}/transfer`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function completeTransfer(callId) {
  return apiFetch(`/api/v1/calls/${callId}/transfer/complete`, { method: "POST" });
}

export function requestVideo(callId) {
  return apiFetch(`/api/v1/calls/${callId}/video/request`, { method: "POST" });
}

export function respondVideo(callId, accept) {
  return apiFetch(`/api/v1/calls/${callId}/video/respond`, {
    method: "POST",
    body: JSON.stringify({ accept }),
  });
}

export function downgradeVideo(callId) {
  return apiFetch(`/api/v1/calls/${callId}/video/downgrade`, { method: "POST" });
}

export function screenShare(callId, on, legId) {
  return apiFetch(`/api/v1/calls/${callId}/screen-share`, {
    method: "POST",
    body: JSON.stringify({ on, leg_id: legId }),
  });
}

export function sendDtmf(callId, legId, digit) {
  return apiFetch(`/api/v1/calls/${callId}/dtmf`, {
    method: "POST",
    body: JSON.stringify({ leg_id: legId, digit }),
  });
}

export function wrapUp(callId, notes) {
  return apiFetch(`/api/v1/calls/${callId}/wrap-up`, {
    method: "POST",
    body: JSON.stringify({ notes }),
  });
}

export function createGuestSession(queueId, ttlSec = 3600, allowedMedia = "audio") {
  return apiFetch("/api/v1/guest/sessions", {
    method: "POST",
    body: JSON.stringify({ queue_id: queueId, ttl_sec: ttlSec, allowed_media: allowedMedia }),
  });
}

export function guestJoinToken(token, sessionType = "") {
  return apiFetch("/api/v1/guest/join", {
    method: "POST",
    auth: false,
    body: JSON.stringify({ token, session_type: sessionType }),
  });
}

export function fetchTurn(callId) {
  return apiFetch(`/api/v1/calls/${callId}/turn-credentials`);
}

export function listAgents() {
  return apiFetch("/api/v1/agents");
}

export function fetchLiveReport() {
  return apiFetch("/api/v1/reports/live");
}

export function fetchHistoricalReport() {
  return apiFetch(`/api/v1/reports/historical?${todayDateRange()}`);
}

function todayDateRange() {
  const today = formatDate();
  return new URLSearchParams({ from: today, to: today });
}

// 报表范围采用 UTC 当日零点及统一时间格式。
function todayReportRange() {
  const now = new Date();
  const start = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  const end = new Date(Math.max(now.getTime(), start.getTime() + 1));
  return new URLSearchParams({ from: formatDateTime(start), to: formatDateTime(end) });
}

export function listRecordings(callId) {
  const q = callId ? `?call_id=${callId}` : "";
  return apiFetch(`/api/v1/recordings${q}`);
}

export function listAudit() {
  return apiFetch("/api/v1/audit/logs?page=1&page_size=50");
}

export function createIvrFlow(name, draft) {
  return apiFetch("/api/v1/ivr/flows", { method: "POST", body: JSON.stringify({ name, draft }) });
}

export function publishIvr(flowId) {
  return apiFetch(`/api/v1/ivr/flows/${flowId}/publish`, { method: "POST" });
}

export function listIvr() {
  return apiFetch("/api/v1/ivr/flows");
}

export function getIvrFlow(id) {
  return apiFetch(`/api/v1/ivr/flows/${encodeURIComponent(id)}`);
}

export function updateIvrFlow(id, name, draft) {
  return apiFetch(`/api/v1/ivr/flows/${encodeURIComponent(id)}`, { method: "PATCH", body: JSON.stringify({ name, draft }) });
}

export function deleteIvrFlow(id) {
  return apiFetch(`/api/v1/ivr/flows/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export function listIvrVersions(id) {
  return apiFetch(`/api/v1/ivr/flows/${encodeURIComponent(id)}/versions`);
}

export function rollbackIvrFlow(id, version) {
  return apiFetch(`/api/v1/ivr/flows/${encodeURIComponent(id)}/rollback`, { method: "POST", body: JSON.stringify({ version }) });
}

export function listIvrAssets() {
  return apiFetch("/api/v1/ivr-assets");
}

export function uploadIvrAsset(file) {
  const body = new FormData();
  body.append("file", file);
  return apiFetch("/api/v1/ivr-assets", { method: "POST", body, timeoutMs: 120000 });
}

export function fetchIvrAsset(id) {
  return apiFetch(`/api/v1/ivr-assets/${encodeURIComponent(id)}`, { responseType: "blob", timeoutMs: 120000 });
}

export function createWebhook(url, eventTypes) {
  return apiFetch("/api/v1/webhooks/subscriptions", {
    method: "POST",
    body: JSON.stringify({ url, event_types: eventTypes }),
  });
}

export function listWebhooks() {
  return apiFetch("/api/v1/webhooks/subscriptions");
}

export function conferenceInvite(callId, agentId) {
  return apiFetch(`/api/v1/calls/${callId}/conference`, {
    method: "POST",
    body: JSON.stringify({ agent_id: agentId }),
  });
}

export function patchQueue(queueId, body) {
  return apiFetch(`/api/v1/queues/${queueId}`, { method: "PATCH", body: JSON.stringify(body) });
}

export function fetchAgentUtil() {
  return apiFetch(`/api/v1/reports/agents?${todayReportRange()}`);
}

export function listDids() {
  return apiFetch(" /api/v1/dids".trim());
}

export function upsertDid(body) {
  return apiFetch("/api/v1/dids", { method: "POST", body: JSON.stringify(body) });
}

export function addQaMark(callId, offsetSec, label) {
  return apiFetch(`/api/v1/calls/${callId}/qa-marks`, {
    method: "POST",
    body: JSON.stringify({ offset_sec: offsetSec, label }),
  });
}

export function forceCheckout(agentId, policy = "force_hangup") {
  return apiFetch(`/api/v1/supervisor/agents/${agentId}/force-check-out`, {
    method: "POST",
    body: JSON.stringify({ policy }),
  });
}

export function listenCall(callId) {
  return apiFetch(`/api/v1/supervisor/calls/${callId}/listen`, { method: "POST" });
}

export function createSkill(name) {
  return apiFetch("/api/v1/skills", { method: "POST", body: JSON.stringify({ name }) });
}

export function listSkills() {
  return apiFetch("/api/v1/skills");
}

export function bindAgentSkills(agentId, skillIds) {
  return apiFetch(`/api/v1/agents/${agentId}/skills`, {
    method: "PUT",
    body: JSON.stringify({ skill_ids: skillIds }),
  });
}

export function listWrapUps(callId) {
  const q = callId ? `?call_id=${encodeURIComponent(callId)}` : "";
  return apiFetch(`/api/v1/wrap-ups${q}`);
}

export function fetchRecordingBlob(id, format = "") {
 const query = format ? `?format=${encodeURIComponent(format)}` : "";
 return request(`/api/v1/recordings/${id}/download${query}`, { responseType: "blob", timeoutMs: 600000 });
}

export async function downloadRecording(id, format = "") {
  const blob = await fetchRecordingBlob(id, format);
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `${id}${extFromType(blob.type)}`;
  a.click();
  URL.revokeObjectURL(url);
}

function extFromType(type) {
  if (!type) return ".ogg";
  if (type.includes("webm")) return ".webm";
  if (type.includes("mp4")) return ".mp4";
  if (type.includes("ogg")) return ".ogg";
  if (type.includes("wav")) return ".wav";
  if (type.includes("ivf")) return ".ivf";
  return ".bin";
}

export async function downloadCdrCsv() {
  const range = new URLSearchParams({ from: "2020-01-01 00:00:00", to: "2099-01-01 00:00:00" });
  const blob = await request(`/api/v1/cdr/export.csv?${range}`, { responseType: "blob", timeoutMs: 120000 });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "cdr.csv";
  a.click();
  URL.revokeObjectURL(url);
}
