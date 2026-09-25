import { getAccessToken, clearAccessToken } from "./auth-store.js";
import { createId, getCallContext } from "./call-context.js";
import { reportEvent } from "./observability.js";
import { getRuntimeConfig } from "./runtime-config.js";

export const apiEvents = new EventTarget();

export class ApiError extends Error {
  constructor(status, message, body, code = "REQUEST_FAILED", requestId = "", traceId = "") {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
    this.code = body?.code || code;
    this.requestId = body?.request_id || requestId;
    this.traceId = body?.trace_id || traceId;
  }
}

const messages = { 401: "登录已失效，请重新登录", 403: "没有操作权限", 404: "请求的资源不存在", 429: "请求过于频繁，请稍后重试", 500: "服务器内部错误", 502: "后端服务暂不可用", 503: "服务暂不可用" };

// JSON 与文件请求共用此入口；写请求不会自动重试，以免重复执行。
export async function request(path, options = {}) {
  const { auth = true, responseType = "json", timeoutMs = 30000, signal, ...init } = options;
  const { apiBase } = getRuntimeConfig();
  const base = new URL(apiBase || window.location.origin, window.location.origin);
  const url = new URL(/^https?:\/\//i.test(path) ? path : `${base.href.replace(/\/$/, "")}/${path.replace(/^\//, "")}`);
  if (url.origin !== base.origin) throw new ApiError(0, "请求地址不属于配置的后端", null, "INVALID_URL");
  const token = auth ? getAccessToken() : null;
  const headers = new Headers(init.headers);
  const context = getCallContext();
  const requestId = createId();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  headers.set("X-Trace-ID", context.trace_id);
  headers.set("X-Request-ID", requestId);
  if (context.call_id) headers.set("X-Call-ID", context.call_id);
  if (context.leg_id) headers.set("X-Leg-ID", context.leg_id);
  if (init.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  headers.set("Accept", responseType === "blob" ? "*/*" : "application/json");
  const controller = new AbortController();
  let timedOut = false;
  const abort = () => controller.abort();
  if (signal?.aborted) abort();
  signal?.addEventListener("abort", abort, { once: true });
  const timer = setTimeout(() => { timedOut = true; abort(); }, timeoutMs);
  let responseRequestId = "";
  let responseTraceId = "";
  const startedAt = Date.now();
  try {
    const res = await fetch(url, { ...init, headers, signal: controller.signal });
    responseRequestId = res.headers.get("X-Request-ID") || requestId;
    responseTraceId = res.headers.get("X-Trace-ID") || "";
    if (responseTraceId) apiEvents.dispatchEvent(new CustomEvent("trace", { detail: { traceId: responseTraceId, requestId: responseRequestId } }));
    if (responseType === "blob" && res.ok) return await res.blob();
    const text = await res.text();
    let body = null;
    if (text) {
      try { body = JSON.parse(text); }
      catch { if (res.ok && path !== "/health") throw new ApiError(res.status, "服务器返回格式异常", null, "INVALID_RESPONSE", responseRequestId, responseTraceId); }
    }
    if (!responseTraceId && body?.trace_id) {
      responseTraceId = String(body.trace_id);
      apiEvents.dispatchEvent(new CustomEvent("trace", { detail: { traceId: responseTraceId, requestId: responseRequestId } }));
    }
    const envelope = body && typeof body === "object" && typeof body.code === "string" && Object.hasOwn(body, "data");
    if (!res.ok || (envelope && body.code !== "OK")) {
      const error = new ApiError(res.status, body?.message || messages[res.status] || "请求失败", body, "REQUEST_FAILED", responseRequestId, responseTraceId);
      // 旧请求的 401 不能清除之后更新的登录令牌。
      if (res.status === 401 && token && token === getAccessToken()) {
        clearAccessToken();
        error.message = messages[401];
        apiEvents.dispatchEvent(new CustomEvent("unauthorized", { detail: error }));
      }
      throw error;
    }
    reportEvent("http.completed", { code: body?.code || String(res.status), duration_ms: Date.now() - startedAt });
    return envelope ? body.data : (body ?? (text || null));
  } catch (cause) {
    const error = cause instanceof ApiError ? cause : new ApiError(0,
      timedOut ? "请求超时，请稍后重试" : controller.signal.aborted ? "请求已取消" : "网络连接失败，请检查网络或服务状态",
      null, timedOut ? "TIMEOUT" : controller.signal.aborted ? "CANCELED" : "NETWORK_ERROR", responseRequestId || requestId, responseTraceId);
    reportEvent("http.failed", { code: error.code, duration_ms: Date.now() - startedAt, error_name: error.name });
    if (error.code !== "CANCELED") apiEvents.dispatchEvent(new CustomEvent("error", { detail: error }));
    throw error;
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener("abort", abort);
  }
}

// 组件统一订阅请求错误，事件回调中发起的请求也能显示一致的错误提示。
export function bindApiFeedback(component, onUnauthorized) {
  const onError = event => { component.error = event.detail.message; };
  const onAuth = event => { onUnauthorized?.(); component.error = event.detail.message; };
  apiEvents.addEventListener("error", onError);
  apiEvents.addEventListener("unauthorized", onAuth);
  return () => {
    apiEvents.removeEventListener("error", onError);
    apiEvents.removeEventListener("unauthorized", onAuth);
  };
}
