// 坐席档案、签入状态与技能绑定。
import { apiFetch } from "./client.js";

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

export function listAgents() {
  return apiFetch("/api/v1/agents");
}

export function bindAgentSkills(agentId, skillIds) {
  return apiFetch(`/api/v1/agents/${agentId}/skills`, {
    method: "PUT",
    body: JSON.stringify({ skill_ids: skillIds }),
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
