// 本文件负责 Switch 呼入号码路由草稿与发布。
import { html } from "lit";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderField } from "../../shared/components/panel.js";
import { renderPageCrud } from "../../shared/components/page-layout.js";

function targetLabel(d, state) {
  if (!d.target_id) return "";
  if (d.target_type === "queue") {
    const q = (state.queues || []).find((x) => x.id === d.target_id);
    return q ? `${q.name}` : d.target_id;
  }
  if (d.target_type === "ivr") {
    const f = (state.ivrs || []).find((x) => x.id === d.target_id);
    return f ? `${f.name}` : d.target_id;
  }
  return d.target_id;
}

function renderTargetSelect(state, actions) {
  const type = state.didForm.target_type;
  if (type === "reject") {
    return html`<select disabled><option value="">无需选择</option></select>`;
  }
  if (type === "ivr") {
    const published = (state.ivrs || []).filter((f) => f.published_version);
    return html`<select .value=${state.didForm.target_id} @change=${(e) => actions.updateDidForm({ target_id: e.target.value })}>
      <option value="">选择已发布 IVR</option>
      ${published.map((f) => html`<option value=${f.id} ?selected=${f.id === state.didForm.target_id}>${f.name}（v${f.published_version}）</option>`)}
    </select>`;
  }
  const voiceQueues = (state.queues || []).filter((q) => !q.video_enabled);
  return html`<select .value=${state.didForm.target_id} @change=${(e) => actions.updateDidForm({ target_id: e.target.value })}>
    <option value="">选择语音队列</option>
    ${voiceQueues.map((q) => html`<option value=${q.id} ?selected=${q.id === state.didForm.target_id}>${q.name}</option>`)}
  </select>`;
}

export function renderDids(state, actions) {
  const canWrite = state.me?.permissions?.includes("dids.write");
  return renderPageCrud({
    description: "保存仅更新草稿。发布会将 DID、队列、坐席、工作时间和 IVR 快照整体激活；正在进行的通话继续使用原版本。同中继 + 同 DID 再次保存会覆盖原路由。电话呼入只能进语音队列。",
    sections: [{
      title: "呼入号码路由",
      form: canWrite ? html`<form @submit=${(e) => actions.saveDid(e)}>
        <div class="form-inline">
          ${renderField("中继 ID", html`<input .value=${state.didForm.trunk_id} placeholder="*" @input=${(e) => actions.updateDidForm({ trunk_id: e.target.value })} />`, { hint: "来电路由；空或 * 表示任意中继" })}
          ${renderField("DID", html`<input .value=${state.didForm.did} @input=${(e) => actions.updateDidForm({ did: e.target.value })} />`, { hint: "被叫号码；保存后按数字归一化" })}
          ${renderField("目标类型", html`<select .value=${state.didForm.target_type} @change=${(e) => actions.updateDidForm({ target_type: e.target.value, target_id: "" })}>
            <option value="queue">队列</option>
            <option value="ivr">IVR</option>
            <option value="reject">拒绝</option>
          </select>`, { hint: "进队列 / 走 IVR / 拒绝呼入" })}
          ${renderField("目标", renderTargetSelect(state, actions), { hint: "队列仅列语音队列；或选已发布 IVR；拒绝时无需选择" })}
          <button type="submit">保存</button>
        </div>
      </form>` : null,
      list: renderDataTable([
        { label: "DID", render: (d) => d.did },
        { label: "中继", render: (d) => d.trunk_id || "*" },
        { label: "目标类型", render: (d) => d.target_type || "" },
        { label: "目标", render: (d) => targetLabel(d, state) },
      ], state.dids, { emptyMessage: "暂无 DID", showCount: true, label: "呼入号码" }),
    }],
  });
}
