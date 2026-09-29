// 活跃通话与桥接运维（直控 API，需 calls.operate）。
import { html } from "lit";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderField, renderPanel } from "../../shared/components/panel.js";

export function renderRuntime(state, actions) {
  const legs = state.runtimeCall?.legs || [];
  const canOperate = state.me?.permissions?.includes("calls.operate");
  return html`
    ${state.me?.permissions?.includes("calls.read") ? renderPanel("活跃通话", html`
      <div class="form-inline">
        <button class="secondary" @click=${() => actions.loadRuntime()}>刷新列表</button>
      </div>
      ${renderDataTable([
        { label: "通话 ID", key: "id" },
        { label: "状态", key: "state" },
        { label: "主叫", key: "caller" },
        { label: "坐席", key: "agent_id" },
        { label: "版本", key: "version" },
        {
          label: "操作",
          render: (row) => html`<button class="secondary" @click=${() => actions.selectRuntimeCall(row.id)}>桥接</button>`,
        },
      ], state.openCalls || [], { showCount: true, label: "Switch 未结束通话" })}
    `) : ""}
    ${state.runtimeCall ? renderPanel("桥接编辑", html`
      <p class="muted">通话 <code>${state.runtimeCall.id}</code> · 状态 ${state.runtimeCall.state} · version ${state.runtimeCall.version ?? "—"}</p>
      ${renderDataTable([
        { label: "腿 ID", key: "id" },
        { label: "角色", key: "role" },
        { label: "坐席", key: "agent_id" },
      ], legs, { label: "媒体腿" })}
      ${canOperate ? html`
        <h4 class="section-spaced">新建双桥</h4>
        <div class="form-inline">
          ${renderField("腿 A", html`<select @change=${(e) => actions.setBridgeForm({ leg_a: e.target.value })}>
            <option value="">选择</option>
            ${legs.map((l) => html`<option value=${l.id} ?selected=${l.id === state.bridgeForm.leg_a}>${l.id} (${l.role || ""})</option>`)}
          </select>`)}
          ${renderField("腿 B", html`<select @change=${(e) => actions.setBridgeForm({ leg_b: e.target.value })}>
            <option value="">选择</option>
            ${legs.map((l) => html`<option value=${l.id} ?selected=${l.id === state.bridgeForm.leg_b}>${l.id} (${l.role})</option>`)}
          </select>`)}
          <button @click=${() => actions.createBridge()}>建立桥接</button>
        </div>
        <h4 class="section-spaced">替换已有桥（PUT）</h4>
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
          <button class="secondary" @click=${() => actions.replaceBridge()}>替换参与腿</button>
          <button class="danger" @click=${() => actions.endBridge()}>拆桥</button>
        </div>
        <p class="muted">bridge_id 来自 Switch 事件 <code>bridge.active</code> 或数据库 <code>os_bridges</code>；直控场景通常在首次 POST bridges 的响应事件中获取。</p>
      ` : html`<p class="muted">需要 calls.operate 权限才能修改桥接。</p>`}
    `) : ""}
  `;
}
