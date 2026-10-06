import { apiFetch } from "./client.js";

export function fetchAibotIVRTemplate(queueId) {
  const q = queueId ? `?queue_id=${encodeURIComponent(queueId)}` : "";
  return apiFetch(`/api/v1/aibot/ivr-template${q}`);
}

export function getAibotQueueProfile(queueId) {
  return apiFetch(`/api/v1/aibot/queues/${encodeURIComponent(queueId)}/profile`);
}

export function putAibotQueueProfile(queueId, systemPrompt) {
  return apiFetch(`/api/v1/aibot/queues/${encodeURIComponent(queueId)}/profile`, {
    method: "PUT",
    body: JSON.stringify({ system_prompt: systemPrompt }),
  });
}
