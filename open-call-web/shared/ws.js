/**
 * 业务 WebSocket 客户端：连接、重连与 JSON 消息分发（不承载 SDP）。
 */

import { getAccessToken } from "./auth-store.js";
import { reportEvent } from "./observability.js";
import { getRuntimeConfig } from "./runtime-config.js";

/** @typedef {(msg: { type: string, [key: string]: unknown }) => void} WsListener */

export class BusinessWebSocket {
  /** @type {WebSocket|null} */
  #socket = null;
  /** @type {Set<WsListener>} */
  #listeners = new Set();
  #closedByUser = false;
  #pingTimer = null;
  /** @type {ReturnType<typeof setTimeout>|null} */
  #reconnectTimer = null;
  #retry = 0;
  #token = "";
  #lastSeq = 0;
  /** @type {number} */
  #generation = 0;

  /** 使用当前令牌建立业务事件连接，并从新会话开始记录事件序号。 */
  connect(token) {
    this.disconnect();
    const access = token ?? getAccessToken();
    if (!access) {
      throw new Error("WebSocket 需要 access_token 或 guest token");
    }
    this.#token = access;
    this.#lastSeq = 0;
    this.#closedByUser = false;
    this.#open();
    return this;
  }

  /** 重连时携带最后收到的序号，以便服务端补发遗漏事件。 */
  #open() {
    const gen = ++this.#generation;
    const { apiBase, wsPath } = getRuntimeConfig();
    const wsBase = apiBase.replace(/^http/i, (m) => (m.toLowerCase() === "https" ? "wss" : "ws"));
    const base = `${wsBase.replace(/\/$/, "")}${wsPath}`;
    const url = this.#lastSeq > 0 ? `${base}${base.includes("?") ? "&" : "?"}since=${this.#lastSeq}` : base;
    reportEvent("ws.connecting", { attempt: this.#retry + 1 });
    const token = this.#token || getAccessToken();
    const socket = new WebSocket(url, [`open-voip.auth.${token}`]);
    this.#socket = socket;
    socket.addEventListener("open", () => {
      if (gen !== this.#generation || socket !== this.#socket) return;
      reportEvent("ws.open", { attempt: this.#retry + 1 });
      this.#retry = 0;
      this.#pingTimer = setInterval(() => {
        reportEvent("ws.ping");
        this.send("ping", {});
      }, 30000);
    });
    socket.addEventListener("message", (ev) => {
      if (gen !== this.#generation || socket !== this.#socket) return;
      try {
        const msg = JSON.parse(String(ev.data));
        if (Number.isSafeInteger(msg.seq) && this.#lastSeq > 0 && msg.seq > this.#lastSeq + 1) {
          reportEvent("ws.gap", { expected_seq: this.#lastSeq + 1, seq: msg.seq });
        }
        if (Number.isSafeInteger(msg.seq) && msg.seq > this.#lastSeq) this.#lastSeq = msg.seq;
        this.#listeners.forEach((fn) => fn(msg));
      } catch {
        /* 忽略非 JSON */
      }
    });
    socket.addEventListener("close", (event) => {
      if (gen !== this.#generation) return;
      clearInterval(this.#pingTimer);
      this.#pingTimer = null;
      if (this.#socket === socket) this.#socket = null;
      reportEvent("ws.close", { code: event.code, reason: event.reason || "", state: this.#closedByUser ? "user" : "remote" });
      if (!this.#closedByUser) {
        const delay = Math.min(15000, 500 * 2 ** this.#retry);
        this.#retry += 1;
        reportEvent("ws.reconnect", { attempt: this.#retry, delay_ms: delay });
        clearTimeout(this.#reconnectTimer);
        this.#reconnectTimer = setTimeout(() => {
          if (!this.#closedByUser) {
            this.#token = getAccessToken() || this.#token;
            this.#open();
          }
        }, delay);
      }
    });
  }

  /** 连接已就绪时发送业务命令。 */
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

  /** 主动关闭连接并清除心跳，避免触发自动重连。 */
  disconnect() {
    this.#closedByUser = true;
    this.#generation += 1;
    clearInterval(this.#pingTimer);
    this.#pingTimer = null;
    clearTimeout(this.#reconnectTimer);
    this.#reconnectTimer = null;
    this.#socket?.close();
    this.#socket = null;
  }
}
