import { LitElement, html } from "lit";
import { fetchHealth, fetchStatus } from "../shared/api.js";
import { shellStyles } from "../shared/shell-styles.js";
import { isWebRTCSupported } from "../shared/webrtc.js";

export class AgentApp extends LitElement {
  static properties = {
    health: { type: String },
    status: { type: Object },
    error: { type: String },
  };

  static styles = shellStyles;

  constructor() {
    super();
    this.health = "";
    this.status = null;
    this.error = "";
  }

  connectedCallback() {
    super.connectedCallback();
    this.#probe();
  }

  async #probe() {
    try {
      this.health = await fetchHealth();
      this.status = await fetchStatus();
      this.error = "";
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  render() {
    return html`
      <header>
        <strong>坐席端</strong>
        <span class="badge">Phase 0 框架</span>
      </header>
      <main>
        <p>WebRTC：${isWebRTCSupported() ? "支持" : "不支持"}</p>
        <p>健康检查：<code>${this.health || "…"}</code></p>
        ${this.status
          ? html`<pre>${JSON.stringify(this.status, null, 2)}</pre>`
          : ""}
        ${this.error ? html`<p class="error">${this.error}</p>` : ""}
        <p>后续 Sprint：登录、签入、WebSocket 振铃与通话 UI 将在此组件扩展。</p>
      </main>
    `;
  }
}

customElements.define("open-voip-agent-app", AgentApp);
