// 本文件负责Webhook 管理页，展示订阅和投递记录。
import { html } from "lit";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderField, renderPanel } from "../../shared/components/panel.js";
export function renderHooks(state, actions) {
    const rows = Array.isArray(state.hooks) ? state.hooks : [];
    return renderPanel("Webhook", html`
        ${state.me?.permissions?.includes("webhooks.write") ? html`<div class="form-inline">
          ${renderField("回调 URL", html`<input .value=${state.hookUrl} @input=${(e) => actions.setHookUrl(e.target.value)} />`, { className: "field-wide" })}
          <button class="secondary" @click=${() => actions.addHook()}>订阅全部事件</button>
        </div>` : ""}
        ${renderDataTable([
          { label: "URL", render: (h) => h.url || h.URL || "" },
          { label: "启用", render: (h) => h.enabled === false ? "否" : "是" },
        ], rows, { label: "Webhook 订阅" })}
        <pre>${JSON.stringify(state.hooks, null, 2)}</pre>
    `);
  }
