// 活跃通话与桥接运维（直控 API，需 calls.operate）。
import { html, nothing } from "lit";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderField, renderPanel } from "../../shared/components/panel.js";
import { renderCrudSection, renderPageSplit } from "../../shared/components/page-layout.js";

export function renderRuntime(state, actions) {
  const legs = state.runtimeCall?.legs || [];
  const canOperate = state.me?.permissions?.includes("calls.operate");
  const canRead = state.me?.permissions?.includes("calls.read");

  const master = canRead ? renderCrudSection({
    title: "活跃通话",
    form: html`<div class="form-inline">
      <button type="button" class="secondary" @click=${() => actions.loadRuntime()}>刷新列表</button>
    </div>`,
    list: renderDataTable([
      { label: "通话 ID", key: "id" },
      { label: "状态", key: "state" },
      { label: "主叫", key: "caller" },
      { label: "坐席", key: "agent_id" },
      { label: "版本", key: "version" },
      {
        label: "操作",
        render: (row) => html`<button type="button" class="secondary" @click=${() => actions.selectRuntimeCall(row.id)}>桥接</button>`,
      },
    ], state.openCalls || [], { showCount: true, label: "Switch 未结束通话", emptyMessage: "暂无活跃通话" }),
  }) : nothing;

  const detail = state.runtimeCall ? renderCrudSection({
    title: "桥接编辑",
    hint: `通话 ${state.runtimeCall.id} · 状态 ${state.runtimeCall.state} · version ${state.runtimeCall.version ?? "—"}`,
    list: renderDataTable([
      { label: "腿 ID", key: "id" },
      { label: "角色", key: "role" },
      { label: "坐席", key: "agent_id" },
    ], legs, { label: "媒体腿", emptyMessage: "暂无媒体腿" }),
    form: canOperate ? html`
      <h4 class="section-title">新建双桥</h4>
      <div class="form-inline">
        ${renderField("腿 A", html`<select @change=${(e) => actions.setBridgeForm({ leg_a: e.target.value })}>
          <option value="">选择</option>
          ${legs.map((l) => html`<option value=${l.id} ?selected=${l.id === state.bridgeForm.leg_a}>${l.id} (${l.role || ""})</option>`)}
        </select>`)}
        ${renderField("腿 B", html`<select @change=${(e) => actions.setBridgeForm({ leg_b: e.target.value })}>
          <option value="">选择</option>
          ${legs.map((l) => html`<option value=${l.id} ?selected=${l.id === state.bridgeForm.leg_b}>${l.id} (${l.role})</option>`)}
        </select>`)}
        <button type="button" @click=${() => actions.createBridge()}>建立桥接</button>
      </div>
      <h4 class="section-title">替换已有桥（PUT）</h4>
      <div class="form-inline">
        ${renderField("bridge_id", html`<input .value=${state.bridgeForm.bridge_id || ""} @input=${(e) => actions.setBridgeForm({ bridge_id: e.target.value })} placeholder="os_bridges.id" />`)}
        ${renderField("腿 A", html`<select @change=${(e) => actions.setBridgeForm({ leg_a: e.target.value })}>
          <option value="">选择</option>
          ${legs.map((l) => html`<option value=${l.id}>${l.id}</option>`)}
        </select>`)}
        ${renderField("腿 B", html`<select @change=${(e) => actions.setBridgeForm({ leg_b: e.target.value })}>
          <option value="">选择</option>
          ${legs.map((l) => html`<option value=${l.id}>${l.id}</option>`)}
        </select>`)}
        <button type="button" class="secondary" @click=${() => actions.replaceBridge()}>替换参与腿</button>
        <button type="button" class="danger" @click=${() => actions.endBridge()}>拆桥</button>
      </div>
      <p class="muted">bridge_id 来自 Switch 事件 bridge.active 或数据库 os_bridges；直控场景通常在首次 POST bridges 的响应事件中获取。</p>
    ` : html`<p class="muted">需要 calls.operate 权限才能修改桥接。</p>`,
  }) : renderPanel("桥接编辑", html`<p class="muted">从左侧选择一通活跃通话以编辑桥接。</p>`);

  return renderPageSplit({
    master,
    detail,
  });
}
