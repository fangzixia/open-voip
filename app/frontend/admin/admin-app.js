import { LitElement, html } from "lit";
import { fetchStatus } from "../shared/api.js";
import { shellStyles } from "../shared/shell-styles.js";

export class AdminApp extends LitElement {
  static properties = {
    status: { type: Object },
    error: { type: String },
  };

  static styles = shellStyles;

  constructor() {
    super();
    this.status = null;
    this.error = "";
  }

  connectedCallback() {
    super.connectedCallback();
    fetchStatus()
      .then((s) => {
        this.status = s;
      })
      .catch((e) => {
        this.error = e instanceof Error ? e.message : String(e);
      });
  }

  render() {
    return html`
      <header>
        <strong>管理端</strong>
        <span class="badge">Phase 0 框架</span>
      </header>
      <main>
        ${this.status
          ? html`<pre>${JSON.stringify(this.status, null, 2)}</pre>`
          : html`<p>加载状态…</p>`}
        ${this.error ? html`<p class="error">${this.error}</p>` : ""}
        <p>后续：队列/坐席 CRUD 与 OpenAPI 对齐的表单将在此扩展。</p>
      </main>
    `;
  }
}

customElements.define("open-voip-admin-app", AdminApp);
