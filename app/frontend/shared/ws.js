/**
 * 业务 WebSocket 客户端：连接、重连与 JSON 消息分发（不承载 SDP）。
 */

import { getAccessToken } from "./auth-store.js";
import { getRuntimeConfig } from "./runtime-config.js";

/** @typedef {(msg: { type: string, [key: string]: unknown }) => void} WsListener */

export class BusinessWebSocket {
  /** @type {WebSocket|null} */
  #socket = null;
  /** @type {Set<WsListener>} */
  #listeners = new Set();
  #closedByUser = false;

  connect(token) {
    const access = token ?? getAccessToken();
    if (!access) {
      throw new Error("WebSocket 需要 access_token 或 guest token");
    }
    const { apiBase, wsPath } = getRuntimeConfig();
    const wsBase = apiBase.replace(/^http/i, (m) => (m.toLowerCase() === "https" ? "wss" : "ws"));
    const url = `${wsBase.replace(/\/$/, "")}${wsPath}?token=${encodeURIComponent(access)}`;
    this.#closedByUser = false;
    this.#socket = new WebSocket(url);
    this.#socket.addEventListener("message", (ev) => {
      try {
        const msg = JSON.parse(String(ev.data));
        this.#listeners.forEach((fn) => fn(msg));
      } catch {
        /* 忽略非 JSON */
      }
    });
    this.#socket.addEventListener("close", () => {
      if (!this.#closedByUser) {
        /* Phase 1：指数退避重连 */
      }
    });
    return this.#socket;
  }

  /** @param {WsListener} fn */
  subscribe(fn) {
    this.#listeners.add(fn);
    return () => this.#listeners.delete(fn);
  }

  disconnect() {
    this.#closedByUser = true;
    this.#socket?.close();
    this.#socket = null;
  }
}
