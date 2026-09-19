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
  #pingTimer = null;
  #retry = 0;
  #token = "";

  connect(token) {
    this.disconnect();
    const access = token ?? getAccessToken();
    if (!access) {
      throw new Error("WebSocket 需要 access_token 或 guest token");
    }
    this.#token = access;
    this.#closedByUser = false;
    this.#open();
    return this;
  }

  #open() {
    const { apiBase, wsPath } = getRuntimeConfig();
    const wsBase = apiBase.replace(/^http/i, (m) => (m.toLowerCase() === "https" ? "wss" : "ws"));
    const url = `${wsBase.replace(/\/$/, "")}${wsPath}?token=${encodeURIComponent(this.#token)}`;
    this.#socket = new WebSocket(url);
    this.#socket.addEventListener("open", () => {
      this.#retry = 0;
      this.#pingTimer = setInterval(() => this.send("ping", {}), 30000);
    });
    this.#socket.addEventListener("message", (ev) => {
      try {
        const msg = JSON.parse(String(ev.data));
        this.#listeners.forEach((fn) => fn(msg));
      } catch {
        /* 忽略非 JSON */
      }
    });
    this.#socket.addEventListener("close", () => {
      clearInterval(this.#pingTimer);
      this.#pingTimer = null;
      if (!this.#closedByUser) {
        const delay = Math.min(15000, 500 * 2 ** this.#retry);
        this.#retry += 1;
        setTimeout(() => {
          if (!this.#closedByUser) this.#open();
        }, delay);
      }
    });
  }

  send(type, payload = {}) {
    if (this.#socket?.readyState === WebSocket.OPEN) {
      this.#socket.send(JSON.stringify({ type, payload }));
    }
  }

  /** @param {WsListener} fn */
  subscribe(fn) {
    this.#listeners.add(fn);
    return () => this.#listeners.delete(fn);
  }

  disconnect() {
    this.#closedByUser = true;
    clearInterval(this.#pingTimer);
    this.#pingTimer = null;
    this.#socket?.close();
    this.#socket = null;
  }
}
