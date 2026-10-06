// 统一登录壳：坐席与管理共用一个侧栏菜单。
import { LitElement, css, html } from "lit";
import { authMe, authOptions, changePassword, exchangeSSOTicket, login, logout, popSSOTicket, startSSO } from "./shared/api.js";
import { apiEvents } from "./shared/http-client.js";
import { clearAccessToken, getAccessToken, setAuthTokens } from "./shared/auth-store.js";
import { renderCredentialFields } from "./shared/components/credentials.js";
import { renderAppShell, renderLoginLayout } from "./shared/components/ui.js";
import { formatDateTime } from "./shared/datetime.js";
import { appStyles } from "./shared/styles/index.js";
import { availableViews } from "./shared/workspace-permissions.js";
import {
  buildStaffNav,
  defaultStaffNavId,
  isAdminNav,
  isAgentNav,
  staffNavLabel,
} from "./shared/staff-nav.js";
import "./admin/admin-app.js";
import "./agent/agent-app.js";

export class OpenVoIPApp extends LitElement {
  static properties = {
    loginName: { type: String },
    password: { type: String },
    error: { type: String },
    authOptions: { type: Object },
    me: { type: Object },
    activeNav: { type: String },
    mustChangePassword: { type: Boolean },
    newPassword: { type: String },
    clock: { type: String },
    navBadges: { type: Object },
  };

  static styles = [...appStyles, css`
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
    this.activeNav = "";
    this.mustChangePassword = false;
    this.newPassword = "";
    this.clock = "";
    this.navBadges = { inbound: 0 };
    this.onUnauthorized = () => {
      this.me = null;
      this.activeNav = "";
    };
    this.onSessionEnded = () => {
      this.me = null;
      this.activeNav = "";
    };
    this.onStaffNavigate = (event) => {
      const id = event.detail?.id;
      if (id) this.activeNav = id;
    };
    this.onStaffNavBadges = (event) => {
      this.navBadges = { ...this.navBadges, ...event.detail };
    };
  }

  connectedCallback() {
    super.connectedCallback();
    apiEvents.addEventListener("unauthorized", this.onUnauthorized);
    this.addEventListener("session-ended", this.onSessionEnded);
    this.addEventListener("staff-navigate", this.onStaffNavigate);
    this.addEventListener("staff-nav-badges", this.onStaffNavBadges);
    this.#tickClock();
    this.#clockTimer = setInterval(() => this.#tickClock(), 1000);
    authOptions().then((value) => {
      this.authOptions = value || {};
    }).catch(() => {
      this.authOptions = { unavailable: true };
      this.error = "无法读取登录方式";
    });
    const ticket = popSSOTicket();
    if (ticket) void this.completeSSO(ticket);
    else if (getAccessToken()) void this.loadIdentity();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    apiEvents.removeEventListener("unauthorized", this.onUnauthorized);
    this.removeEventListener("session-ended", this.onSessionEnded);
    this.removeEventListener("staff-navigate", this.onStaffNavigate);
    this.removeEventListener("staff-nav-badges", this.onStaffNavBadges);
    clearInterval(this.#clockTimer);
  }

  #clockTimer = null;

  #tickClock() {
    this.clock = formatDateTime();
  }

  async loadIdentity() {
    try {
      const identity = await authMe();
      this.me = identity;
      const navItems = buildStaffNav(identity);
      if (!navItems.some((item) => item.id === this.activeNav)) {
        this.activeNav = defaultStaffNavId(navItems);
      }
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

  async #logout() {
    try {
      await logout();
    } catch {
      /* 仍清除本地会话 */
    } finally {
      clearAccessToken();
      this.me = null;
      this.activeNav = "";
      this.dispatchEvent(new CustomEvent("session-ended", { bubbles: true, composed: true }));
    }
  }

  #onNavigate(id) {
    this.activeNav = id;
  }

  render() {
    if (this.mustChangePassword) {
      return renderLoginLayout({
        subtitle: "Open VoIP",
        title: "修改临时密码",
        hint: "首次登录需要设置新密码，完成后重新登录。",
        onSubmit: (event) => this.submitPasswordChange(event),
        error: this.error,
        fields: html`<label>新密码<input type="password" autocomplete="new-password" .value=${this.newPassword}
          @input=${(event) => { this.newPassword = event.target.value; }} /></label>`,
        submitLabel: "修改密码",
      });
    }
    if (!this.me) {
      return renderLoginLayout({
        subtitle: "Open VoIP",
        title: "登录",
        hint: this.authOptions?.oidc_enabled ? "员工使用统一身份平台；密码入口仅供应急管理员" : "请输入登录名和密码",
        onSubmit: (event) => this.submitLogin(event),
        error: this.error,
        fields: !this.authOptions || this.authOptions.unavailable ? "" : renderCredentialFields({
          prefix: "staff",
          username: this.loginName,
          password: this.password,
          onUsernameChange: (value) => { this.loginName = value; },
          onPasswordChange: (value) => { this.password = value; },
        }),
        showSubmit: !!this.authOptions && !this.authOptions.unavailable,
        extraActions: this.authOptions?.oidc_enabled
          ? html`<button type="button" @click=${() => startSSO()}>统一身份平台登录</button>`
          : "",
      });
    }

    const views = availableViews(this.me);
    const navItems = buildStaffNav(this.me);
    if (!navItems.length) {
      return html`<p class="empty">当前账号没有可用的管理或坐席权限，请联系管理员配置角色和坐席资料。</p>`;
    }
    const label = this.me.display_name || this.me.username || "用户";
    return renderAppShell({
      subtitle: "员工工作台",
      navItems,
      activeNav: this.activeNav,
      onNavigate: (id) => this.#onNavigate(id),
      breadcrumb: staffNavLabel(navItems, this.activeNav),
      badges: this.navBadges,
      topbar: html`
        <span class="topbar-meta">${this.clock}</span>
        <span class="topbar-meta">${label}</span>
        <button type="button" @click=${() => this.#logout()}>退出</button>
      `,
      content: html`
        ${views.agent ? html`
          <open-voip-agent-app
            embedded
            content-only
            staff-nav=${this.activeNav}
            ?hidden=${!isAgentNav(this.activeNav)}
            ?workspaceActive=${!!this.me}
          ></open-voip-agent-app>` : ""}
        ${views.admin ? html`
          <open-voip-admin-app
            embedded
            content-only
            staff-nav=${this.activeNav}
            ?hidden=${!isAdminNav(this.activeNav)}
            ?workspaceActive=${!!this.me}
          ></open-voip-admin-app>` : ""}
      `,
    });
  }
}

customElements.define("open-voip-app", OpenVoIPApp);
