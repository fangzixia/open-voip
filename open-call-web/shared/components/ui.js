// 本文件负责前端通用界面组件。
import { html, nothing } from "lit";

function renderBrand(subtitle) {
  return html`<div class="brand">
    <span class="brand-mark" aria-hidden="true">OV</span>
    <span>Open VoIP</span>
    <span class="brand-sub">${subtitle}</span>
  </div>`;
}

export function renderFeedback({ error = "", notice = "" } = {}) {
  return html`
    ${error ? html`<p class="error" role="alert">${error}</p>` : nothing}
    ${notice ? html`<p class="notice" role="status" aria-live="polite">${notice}</p>` : nothing}
  `;
}

export function renderAppShell({
  subtitle,
  navItems,
  activeNav,
  onNavigate,
  breadcrumb,
  topbar = nothing,
  badges = {},
  content,
  minimal = false,
}) {
  return html`
    <div class="layout">
      <header class="topbar">
        ${renderBrand(subtitle)}
        <span class="spacer"></span>
        ${topbar}
      </header>
      <div class="layout-body">
        ${minimal ? nothing : html`<nav class="sidebar" aria-label="${subtitle}主菜单">
          ${navItems.map(
            (item, index) => html`
              ${item.group && navItems[index - 1]?.group !== item.group
                ? html`<span class="nav-section">${item.group}</span>`
                : nothing}
              <button
                class="nav-item ${activeNav === item.id ? "active" : ""}"
                aria-current=${activeNav === item.id ? "page" : nothing}
                @click=${() => onNavigate(item.id)}
              >
                <span>${item.label}</span>
                ${badges[item.id] ? html`<span class="nav-badge" aria-label="${badges[item.id]} 条提醒">${badges[item.id]}</span>` : nothing}
              </button>
            `,
          )}
        </nav>`}
        <main class="content ${minimal ? "content-minimal" : ""}" id="main-content">
          ${minimal ? nothing : html`<div class="breadcrumb">${subtitle} / <strong>${breadcrumb}</strong></div>`}
          ${content}
        </main>
      </div>
    </div>
  `;
}

export function renderLoginLayout({ subtitle, title, hint, onSubmit, fields, error = "", showSubmit = true, submitLabel = "登录", extraActions = "" }) {
  return html`
    <div class="login-page">
      <header class="topbar">
        ${renderBrand(subtitle)}
      </header>
      <div class="login-wrap">
        <form class="login-card" aria-label="${title}" @submit=${onSubmit}>
          <h2>${title}</h2>
          <p class="hint">${hint}</p>
          ${fields}
          ${showSubmit ? html`<button type="submit">${submitLabel}</button>` : ""}
          ${extraActions}
          ${error ? html`<p class="error login-error" role="alert">${error}</p>` : nothing}
        </form>
      </div>
    </div>
  `;
}

export function renderDialpad(onDigit, { disabled = false } = {}) {
  return html`
    <div class="dialpad" role="group" aria-label="数字拨号盘">
      ${["1", "2", "3", "4", "5", "6", "7", "8", "9", "*", "0", "#"].map(
        (digit) => html`<button type="button" ?disabled=${disabled} aria-label="按键 ${digit}" @click=${() => onDigit(digit)}>${digit}</button>`,
      )}
    </div>
  `;
}
