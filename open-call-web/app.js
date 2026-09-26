import { LitElement, css, html } from "lit";
import { authMe, authOptions, changePassword, exchangeSSOTicket, login, popSSOTicket, startSSO } from "./shared/api.js";
import { apiEvents } from "./shared/http-client.js";
import { clearAccessToken, getAccessToken, setAuthTokens } from "./shared/auth-store.js";
import { renderCredentialFields } from "./shared/components/credentials.js";
import { renderLoginLayout } from "./shared/components/ui.js";
import { appStyles } from "./shared/styles/index.js";
import { availableViews } from "./shared/workspace-permissions.js";
import "./admin/admin-app.js";
import "./agent/agent-app.js";

export class OpenVoIPApp extends LitElement {
  static properties = {
    loginName: { type: String }, password: { type: String }, error: { type: String },
    authOptions: { type: Object }, me: { type: Object }, activeView: { type: String },
    mustChangePassword: { type: Boolean }, newPassword: { type: String },
  };

  static styles = [...appStyles, css`
    .workspace-switch { display: flex; gap: 8px; padding: 10px 20px; background: #fff; border-bottom: 1px solid #d8dee6; }
    .workspace-switch button[aria-current="page"] { font-weight: 700; border-color: #2866c7; color: #174f9e; }
    [hidden] { display: none !important; }
    .empty { padding: 32px; }
  `];

  constructor() {
    super();
    this.loginName = "";
    this.password = "";
    this.error = "";
    this.authOptions = null;
    this.me = null;
    this.activeView = "";
    this.mustChangePassword = false;
    this.newPassword = "";
    this.onUnauthorized = () => { this.me = null; this.activeView = ""; };
    this.onSessionEnded = () => { this.me = null; this.activeView = ""; };
  }

  connectedCallback() {
    super.connectedCallback();
    apiEvents.addEventListener("unauthorized", this.onUnauthorized);
    this.addEventListener("session-ended", this.onSessionEnded);
    authOptions().then((value) => { this.authOptions = value || {}; })
      .catch(() => { this.authOptions = { unavailable: true }; this.error = "无法读取登录方式"; });
    const ticket = popSSOTicket();
    if (ticket) void this.completeSSO(ticket);
    else if (getAccessToken()) void this.loadIdentity();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    apiEvents.removeEventListener("unauthorized", this.onUnauthorized);
    this.removeEventListener("session-ended", this.onSessionEnded);
  }

  async loadIdentity() {
    try {
      const identity = await authMe();
      this.me = identity;
      const views = availableViews(identity);
      if (!views[this.activeView]) this.activeView = views.agent ? "agent" : views.admin ? "admin" : "";
      this.error = "";
    } catch (error) {
      clearAccessToken();
      this.me = null;
      this.error = error instanceof Error ? error.message : String(error);
    }
  }

  async submitLogin(event) {
    event.preventDefault();
    try {
      const tokens = await login(this.loginName, this.password);
      setAuthTokens(tokens, { persist: true });
      if (tokens.must_change_password) {
        this.mustChangePassword = true;
        this.error = "";
        return;
      }
      await this.loadIdentity();
    } catch (error) {
      this.error = error instanceof Error ? error.message : String(error);
    }
  }

  async completeSSO(ticket) {
    try {
      setAuthTokens(await exchangeSSOTicket(ticket), { persist: true });
      await this.loadIdentity();
    } catch (error) {
      this.error = error instanceof Error ? error.message : String(error);
    }
  }

  async submitPasswordChange(event) {
    event.preventDefault();
    try {
      await changePassword(this.password, this.newPassword);
      clearAccessToken();
      this.password = "";
      this.newPassword = "";
      this.mustChangePassword = false;
      this.error = "密码已修改，请重新登录";
    } catch (error) {
      this.error = error instanceof Error ? error.message : String(error);
    }
  }

  render() {
    if (this.mustChangePassword) return renderLoginLayout({
      subtitle: "Open VoIP", title: "修改临时密码",
      hint: "首次登录需要设置新密码，完成后重新登录。",
      onSubmit: (event) => this.submitPasswordChange(event), error: this.error,
      fields: html`<label>新密码<input type="password" autocomplete="new-password" .value=${this.newPassword}
        @input=${(event) => { this.newPassword = event.target.value; }} /></label>`,
      submitLabel: "修改密码",
    });
    if (!this.me) return renderLoginLayout({
      subtitle: "Open VoIP", title: "登录",
      hint: this.authOptions?.oidc_enabled ? "员工使用统一身份平台；密码入口仅供应急管理员" : "请输入登录名和密码",
      onSubmit: (event) => this.submitLogin(event), error: this.error,
      fields: !this.authOptions || this.authOptions.unavailable ? "" : renderCredentialFields({
        prefix: "staff", username: this.loginName, password: this.password,
        onUsernameChange: (value) => { this.loginName = value; },
        onPasswordChange: (value) => { this.password = value; },
      }),
      showSubmit: !!this.authOptions && !this.authOptions.unavailable,
      extraActions: this.authOptions?.oidc_enabled ? html`<button type="button" @click=${() => startSSO()}>统一身份平台登录</button>` : "",
    });
    const views = availableViews(this.me);
    if (!views.admin && !views.agent) return html`<p class="empty">当前账号没有可用的管理或坐席权限，请联系管理员配置角色和坐席资料。</p>`;
    return html`
      ${views.admin && views.agent ? html`<nav class="workspace-switch" aria-label="工作区">
        <button aria-current=${this.activeView === "agent" ? "page" : "false"} @click=${() => { this.activeView = "agent"; }}>坐席工作台</button>
        <button aria-current=${this.activeView === "admin" ? "page" : "false"} @click=${() => { this.activeView = "admin"; }}>管理功能</button>
      </nav>` : ""}
      ${views.agent ? html`<open-voip-agent-app ?hidden=${this.activeView !== "agent"}></open-voip-agent-app>` : ""}
      ${views.admin ? html`<open-voip-admin-app ?hidden=${this.activeView !== "admin"}></open-voip-admin-app>` : ""}
    `;
  }
}

customElements.define("open-voip-app", OpenVoIPApp);
