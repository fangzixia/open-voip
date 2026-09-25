import { createId, getCallContext } from "./call-context.js";
import { getAccessToken } from "./auth-store.js";
import { getRuntimeConfig } from "./runtime-config.js";

const MAX_BATCH = 20;
const MAX_QUEUE = 200;
const FLUSH_MS = 5000;
const MAX_BACKOFF_MS = 60000;
const SAFE_DETAIL_KEYS = new Set([
  "attempt", "code", "delay_ms", "direction", "duration_ms", "error_name",
  "fps", "ice_connection_state", "ice_gathering_state", "kind", "message",
  "packets", "packets_lost", "peer_connection_state", "phase", "reason",
  "received_bytes", "remote_candidate_type", "rtt_ms", "sent_bytes",
  "state", "transport", "bitrate_bps", "jitter_ms", "local_candidate_type",
  "seq", "expected_seq",
]);
const FORBIDDEN_KEY = /token|authorization|password|credential|sdp|candidate($|_text)|device.*label/i;

/** 只保留允许上报的有限字段，避免携带凭据、SDP 或设备标识。 */
export function sanitizeDetails(details = {}) {
  const clean = {};
  if (!details || typeof details !== "object") return clean;
  for (const [key, value] of Object.entries(details)) {
    if (FORBIDDEN_KEY.test(key) || !SAFE_DETAIL_KEYS.has(key)) continue;
    if (typeof value === "string") clean[key] = value.slice(0, 256);
    else if (typeof value === "number" && Number.isFinite(value)) clean[key] = value;
    else if (typeof value === "boolean") clean[key] = value;
  }
  return clean;
}

export class EventReporter {
  constructor({
    fetchImpl = globalThis.fetch?.bind(globalThis),
    endpoint = "/api/v1/client-events",
    setTimer = globalThis.setTimeout?.bind(globalThis),
    clearTimer = globalThis.clearTimeout?.bind(globalThis),
    getToken = getAccessToken,
  } = {}) {
    this.fetchImpl = fetchImpl;
    this.endpoint = endpoint;
    this.setTimer = setTimer;
    this.clearTimer = clearTimer;
    this.getToken = getToken;
    this.queue = [];
    this.timer = null;
    this.sending = false;
    this.backoffMs = 1000;
  }

  /** 将客户端事件加入有界队列，满批次时立即发送。 */
  report(type, details = {}) {
    if (!type || typeof type !== "string") return;
    const ctx = getCallContext();
    this.queue.push({
      type: type.slice(0, 80),
      timestamp: new Date().toISOString(),
      call_id: ctx.call_id,
      leg_id: ctx.leg_id,
      queue_id: ctx.queue_id,
      client_session_id: ctx.client_session_id,
      fields: { event_id: createId(), ...sanitizeDetails(details) },
    });
    if (this.queue.length > MAX_QUEUE) this.queue.splice(0, this.queue.length - MAX_QUEUE);
    if (this.queue.length >= MAX_BATCH) void this.flush();
    else this.#schedule(FLUSH_MS);
  }

  #schedule(delay) {
    if (this.timer) return;
    this.timer = this.setTimer(() => {
      this.timer = null;
      void this.flush();
    }, delay);
    this.timer?.unref?.();
  }

  #url() {
    const { apiBase } = getRuntimeConfig();
    const origin = globalThis.location?.origin || "http://localhost";
    return new URL(this.endpoint, `${apiBase || origin}/`).href;
  }

  /** 成批发送事件；失败时放回队首并按指数退避重试。 */
  async flush({ keepalive = false } = {}) {
    if (this.sending || !this.queue.length || !this.fetchImpl) return false;
    const token = this.getToken();
    if (!token) {
      this.#schedule(FLUSH_MS);
      return false;
    }
    if (this.timer) this.clearTimer(this.timer);
    this.timer = null;
    const batch = this.queue.splice(0, MAX_BATCH);
    this.sending = true;
    try {
      const ctx = getCallContext();
      const headers = new Headers({ "Content-Type": "application/json" });
      headers.set("Authorization", `Bearer ${token}`);
      headers.set("X-Trace-ID", ctx.trace_id);
      headers.set("X-Request-ID", createId());
      headers.set("X-Client-Session-ID", ctx.client_session_id);
      if (ctx.call_id) headers.set("X-Call-ID", ctx.call_id);
      if (ctx.leg_id) headers.set("X-Leg-ID", ctx.leg_id);
      const response = await this.fetchImpl(this.#url(), {
        method: "POST",
        headers,
        body: JSON.stringify({ events: batch }),
        credentials: "same-origin",
        keepalive,
      });
      if (!response.ok) throw new Error(`client events HTTP ${response.status}`);
      this.backoffMs = 1000;
      if (this.queue.length) this.#schedule(0);
      return true;
    } catch {
      this.queue.unshift(...batch);
      if (this.queue.length > MAX_QUEUE) this.queue.length = MAX_QUEUE;
      this.#schedule(this.backoffMs);
      this.backoffMs = Math.min(MAX_BACKOFF_MS, this.backoffMs * 2);
      return false;
    } finally {
      this.sending = false;
    }
  }

  unload() {
    if (!this.queue.length) return;
    void this.flush({ keepalive: true });
  }
}

export const reporter = new EventReporter();
export const reportEvent = (type, details) => reporter.report(type, details);

if (typeof window !== "undefined" && typeof window.addEventListener === "function") {
  window.addEventListener("pagehide", () => reporter.unload());
}
