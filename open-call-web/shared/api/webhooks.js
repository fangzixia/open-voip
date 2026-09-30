// Webhook 订阅。
import { apiFetch } from "./client.js";

export function createWebhook(url, eventTypes) {
  return apiFetch("/api/v1/webhooks/subscriptions", {
    method: "POST",
    body: JSON.stringify({ url, event_types: eventTypes }),
  });
}

export function listWebhooks() {
  return apiFetch("/api/v1/webhooks/subscriptions");
}
