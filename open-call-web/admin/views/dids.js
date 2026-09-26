// 本文件负责呼入号码路由页，维护号码与队列关系。
import { html } from "lit";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderField, renderPanel } from "../../shared/components/panel.js";
export function renderDids(state, actions) {
    return renderPanel("呼入号码 / 外显号码", html`
        ${state.me?.permissions?.includes("dids.write") ? html`<form @submit=${(e) => actions.saveDid(e)}>
          <div class="form-inline">
            ${renderField("DID", html`<input .value=${state.didForm.did} @input=${(e) => actions.updateDidForm({ did: e.target.value })} />`)}
            ${renderField("队列 ID", html`<input .value=${state.didForm.queue_id} @input=${(e) => actions.updateDidForm({ queue_id: e.target.value })} />`)}
            ${renderField("外显名称", html`<input .value=${state.didForm.display_name} @input=${(e) => actions.updateDidForm({ display_name: e.target.value })} />`)}
            <button type="submit">保存</button>
          </div>
        </form>` : ""}
        ${renderDataTable([
          { label: "DID", render: (d) => d.did || d.DID || d.d_id },
          { label: "队列", render: (d) => d.queue_id || d.QueueID || "" },
          { label: "外显", render: (d) => d.display_name || d.DisplayName || "" },
        ], state.dids, { emptyMessage: "暂无 DID", showCount: true, label: "呼入号码" })}
    `);
  }
