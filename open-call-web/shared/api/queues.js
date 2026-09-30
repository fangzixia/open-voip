// 队列配置与坐席绑定。
import { apiFetch } from "./client.js";

export function listQueues(page = 1) {
  return apiFetch(`/api/v1/queues?page=${page}&page_size=100`);
}

export function createQueue(body) {
  return apiFetch("/api/v1/queues", { method: "POST", body: JSON.stringify(body) });
}

export function patchQueue(queueId, body) {
  return apiFetch(`/api/v1/queues/${queueId}`, { method: "PATCH", body: JSON.stringify(body) });
}

export function bindQueueAgents(queueId, agentIds) {
  return apiFetch(`/api/v1/queues/${queueId}/agents`, {
    method: "PUT",
    body: JSON.stringify({ agent_ids: agentIds }),
  });
}
