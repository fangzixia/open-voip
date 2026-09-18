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
 * @param {string} path 以 / 开头的 API 路径，例如 /api/v1/status
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

/** 健康检查，无需鉴权 */
export function fetchHealth() {
  return apiFetch("/health", { auth: false, method: "GET" });
}

/** 运行状态摘要 */
export function fetchStatus() {
  return apiFetch("/api/v1/status", { method: "GET" });
}
