import { LitElement, html } from "lit";
import { fetchHealth } from "../shared/api.js";
import { shellStyles } from "../shared/shell-styles.js";

export class GuestApp extends LitElement {
  static properties = {
    health: { type: String },
    error: { type: String },
  };

  static styles = shellStyles;

  constructor() {
    super();
    this.health = "";
    this.error = "";
  }

  connectedCallback() {
    super.connectedCallback();
    fetchHealth()
      .then((h) => {
        this.health = h;
      })
      .catch((e) => {
        this.error = e instanceof Error ? e.message : String(e);
      });
  }

  render() {
    return html`
      <header>
        <strong>访客端</strong>
        <span class="badge">Phase 0 框架</span>
      </header>
      <main>
        <p>健康检查：<code>${this.health || "…"}</code></p>
        ${this.error ? html`<p class="error">${this.error}</p>` : ""}
        <p>后续：选择语音/视频队列、等待页与通话页将在此扩展。</p>
      </main>
    `;
  }
}

customElements.define("open-voip-guest-app", GuestApp);
