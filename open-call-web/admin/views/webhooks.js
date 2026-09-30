// 本文件负责 Webhook 管理页，展示订阅和投递记录。
import { html } from "lit";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderField } from "../../shared/components/panel.js";
import { renderPageCrud } from "../../shared/components/page-layout.js";

export function renderHooks(state, actions) {
  const rows = Array.isArray(state.hooks) ? state.hooks : [];
  const canWrite = state.me?.permissions?.includes("webhooks.write");
  return renderPageCrud({
    description: "订阅 Switch / 业务事件回调。保存后新事件将推送到配置的 URL。",
    sections: [{
      title: "Webhook 订阅",
      form: canWrite ? html`<div class="form-inline">
        ${renderField("回调 URL", html`<input .value=${state.hookUrl} @input=${(e) => actions.setHookUrl(e.target.value)} />`, { className: "field-wide" })}
        <button type="button" class="secondary" @click=${() => actions.addHook()}>订阅全部事件</button>
      </div>` : null,
      list: renderDataTable([
        { label: "URL", render: (h) => h.url || h.URL || "" },
        { label: "启用", render: (h) => h.enabled === false ? "否" : "是" },
      ], rows, { label: "Webhook 订阅", emptyMessage: "暂无订阅" }),
      footer: rows.length ? html`<pre class="page-raw">${JSON.stringify(state.hooks, null, 2)}</pre>` : null,
    }],
  });
}
