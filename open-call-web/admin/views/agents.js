// 本文件负责坐席管理页，展示坐席并组织管理操作。
import { html } from "lit";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderField, renderPanel } from "../../shared/components/panel.js";
export function renderAgents(state, actions) {
    return html`
      ${state.me?.permissions?.includes("agents.read") ? renderPanel("坐席", html`
        ${state.me?.permissions?.includes("users.create") ? html`<button class="secondary" @click=${() => actions.navigate("identity")}>新增坐席账号</button>` : ""}
        ${renderDataTable([
          { label: "坐席", render: (agent) => agent.display_name || agent.extension || agent.id },
          { label: "分机", render: (agent) => agent.extension || "" },
          { label: "状态", render: (agent) => agent.state || "—" },
        ], state.agents, { showCount: true, label: "坐席列表" })}
      `) : ""}
      ${state.me?.permissions?.includes("agents.force_checkout") ? renderPanel("班长强制签出", html`
        <div class="form-inline">
          ${renderField("坐席", html`<select @change=${(e) => actions.setForceAgentId(e.target.value)}>
              <option value="">选择坐席</option>
              ${state.agents.map((a) => html`<option value=${a.id}>${a.display_name || a.extension} (${a.state})</option>`)}
            </select>`)}
          <button class="danger" @click=${() => actions.force()}>强制签出</button>
        </div>
      `) : ""}
      ${state.me?.permissions?.includes("reports.read") ? renderPanel("坐席利用率", html`
        ${renderDataTable([
          { label: "坐席", key: "agent_id" },
          { label: "空闲", render: (u) => `${Math.round(u.idle_sec)}s` },
          { label: "通话", render: (u) => `${Math.round(u.on_call_sec)}s` },
          { label: "示忙", render: (u) => `${Math.round(u.busy_sec)}s` },
          { label: "利用率", render: (u) => `${((u.utilization || 0) * 100).toFixed(0)}%` },
        ], state.utils, { label: "坐席利用率" })}
        <h4 class="section-spaced">历史报表</h4>
        <pre>${JSON.stringify(state.hist, null, 2)}</pre>
      `) : ""}
    `;
  }
