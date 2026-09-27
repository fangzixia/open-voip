// 本文件负责 Switch 呼入号码路由草稿与发布。
import { html } from "lit";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderField, renderPanel } from "../../shared/components/panel.js";
export function renderDids(state, actions) {
    return renderPanel("呼入号码 / 外显号码", html`
        ${state.me?.permissions?.includes("dids.write") ? html`<form @submit=${(e) => actions.saveDid(e)}>
          <div class="form-inline">
            ${renderField("中继 ID", html`<input .value=${state.didForm.trunk_id} @input=${(e) => actions.updateDidForm({ trunk_id: e.target.value })} />`)}
            ${renderField("DID", html`<input .value=${state.didForm.did} @input=${(e) => actions.updateDidForm({ did: e.target.value })} />`)}
            ${renderField("目标类型", html`<select .value=${state.didForm.target_type} @change=${(e) => actions.updateDidForm({ target_type: e.target.value })}><option value="queue">队列</option><option value="ivr">IVR</option><option value="reject">拒绝</option></select>`)}
            ${renderField("目标 ID", html`<input .value=${state.didForm.target_id} ?disabled=${state.didForm.target_type === "reject"} @input=${(e) => actions.updateDidForm({ target_id: e.target.value })} />`)}
            <button type="submit">保存</button>
            ${state.me?.permissions?.includes("config.write") ? html`<button type="button" @click=${() => actions.publishConfig()}>发布并激活</button>` : ""}
          </div>
        </form>` : ""}
        ${renderDataTable([
          { label: "DID", render: (d) => d.did || d.DID || d.d_id },
          { label: "中继", render: (d) => d.trunk_id || "*" },
          { label: "目标类型", render: (d) => d.target_type || "" },
          { label: "目标 ID", render: (d) => d.target_id || "" },
        ], state.dids, { emptyMessage: "暂无 DID", showCount: true, label: "呼入号码" })}
    `);
  }
