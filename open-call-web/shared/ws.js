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
  #retry = 0;
  #token = "";
  #lastSeq = 0;

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
    const { apiBase, wsPath } = getRuntimeConfig();
    const wsBase = apiBase.replace(/^http/i, (m) => (m.toLowerCase() === "https" ? "wss" : "ws"));
    const base = `${wsBase.replace(/\/$/, "")}${wsPath}`;
    const url = this.#lastSeq > 0 ? `${base}${base.includes("?") ? "&" : "?"}since=${this.#lastSeq}` : base;
    reportEvent("ws.connecting", { attempt: this.#retry + 1 });
    this.#socket = new WebSocket(url, [`open-voip.auth.${this.#token}`]);
    this.#socket.addEventListener("open", () => {
      reportEvent("ws.open", { attempt: this.#retry + 1 });
      this.#retry = 0;
      this.#pingTimer = setInterval(() => {
        reportEvent("ws.ping");
        this.send("ping", {});
      }, 30000);
    });
    this.#socket.addEventListener("message", (ev) => {
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
    this.#socket.addEventListener("close", (event) => {
      clearInterval(this.#pingTimer);
      this.#pingTimer = null;
      reportEvent("ws.close", { code: event.code, reason: event.reason || "", state: this.#closedByUser ? "user" : "remote" });
      if (!this.#closedByUser) {
        // 指数退避限制在 15 秒，主动断开时不再重连。
        const delay = Math.min(15000, 500 * 2 ** this.#retry);
        this.#retry += 1;
        reportEvent("ws.reconnect", { attempt: this.#retry, delay_ms: delay });
        setTimeout(() => {
          if (!this.#closedByUser) this.#open();
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
    clearInterval(this.#pingTimer);
    this.#pingTimer = null;
    this.#socket?.close();
    this.#socket = null;
  }
}
