/**
 * REST 客户端：统一 base URL、Authorization、JSON 与错误解析。
 */

import { getAccessToken } from "./auth-store.js";
import { getRuntimeConfig } from "./runtime-config.js";

export class ApiError extends Error {
  /** @param {number} status */
  constructor(status, message, body) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

/**
 * @param {string} path 以 / 开头的 API 路径
 * @param {RequestInit & { auth?: boolean }} [options]
 */
export async function apiFetch(path, options = {}) {
  const { apiBase } = getRuntimeConfig();
  const url = path.startsWith("http") ? path : `${apiBase.replace(/\/$/, "")}${path}`;
  const headers = new Headers(options.headers || {});
  if (!headers.has("Content-Type") && options.body) {
    headers.set("Content-Type", "application/json");
  }
  const useAuth = options.auth !== false;
  if (useAuth) {
    const token = getAccessToken();
    if (token) headers.set("Authorization", `Bearer ${token}`);
  }

  const res = await fetch(url, { ...options, headers });
  const text = await res.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = text;
    }
  }
  if (!res.ok) {
    const msg =
      (data && typeof data === "object" && data.message) ||
      res.statusText ||
      "请求失败";
    throw new ApiError(res.status, msg, data);
  }
  return data;
}

export function fetchHealth() {
  return apiFetch("/health", { auth: false, method: "GET" });
}

export function fetchStatus() {
  return apiFetch("/api/v1/status", { method: "GET", auth: false });
}

export function login(username, password) {
  return apiFetch("/api/v1/auth/login", {
    method: "POST",
    auth: false,
    body: JSON.stringify({ username, password }),
  });
}

export function logout() {
  return apiFetch("/api/v1/auth/logout", { method: "POST" });
}

export function fetchAgentMe() {
  return apiFetch("/api/v1/agents/me");
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

export function guestJoin(queueId, sessionType, priority = 0) {
  return apiFetch("/api/v1/guest/join", {
    method: "POST",
    auth: false,
    body: JSON.stringify({ queue_id: queueId, session_type: sessionType, priority }),
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

export function guestJoinToken(token, sessionType) {
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
  return apiFetch("/api/v1/reports/historical");
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
  return apiFetch("/api/v1/reports/agents");
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

export async function fetchRecordingBlob(id) {
  const { apiBase } = getRuntimeConfig();
  const token = getAccessToken();
  const res = await fetch(`${apiBase.replace(/\/$/, "")}/api/v1/recordings/${id}/download`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  if (!res.ok) throw new ApiError(res.status, "下载失败");
  return res.blob();
}

export async function downloadRecording(id) {
  const blob = await fetchRecordingBlob(id);
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
  if (type.includes("ogg")) return ".ogg";
  if (type.includes("wav")) return ".wav";
  if (type.includes("ivf")) return ".ivf";
  return ".bin";
}

export async function downloadCdrCsv() {
  const { apiBase } = getRuntimeConfig();
  const token = getAccessToken();
  const res = await fetch(
    `${apiBase.replace(/\/$/, "")}/api/v1/cdr/export.csv?from=2020-01-01T00:00:00Z&to=2099-01-01T00:00:00Z`,
    { headers: token ? { Authorization: `Bearer ${token}` } : {} },
  );
  if (!res.ok) throw new ApiError(res.status, "导出失败");
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "cdr.csv";
  a.click();
  URL.revokeObjectURL(url);
}
