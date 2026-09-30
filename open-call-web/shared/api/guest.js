// 访客排队与邀请会话。
import { apiFetch } from "./client.js";

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
