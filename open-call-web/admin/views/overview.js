// 本文件负责管理概览页，汇总运行状态。
import { html } from "lit";
import { strategyLabel } from "./labels.js";
import { agentStateLabel, agentStateTone } from "../../shared/display.js";
import { renderCdrQuery } from "./cdr.js";
import { renderCdrTable } from "./cdr.js";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderPanel } from "../../shared/components/panel.js";
import { renderStatusTag } from "../../shared/components/status-tag.js";
export function renderOverview(state, actions) {
    const sipOk = state.status?.sip_listening ?? state.status?.sip_ok ?? true;
    return html`
      <div class="kpi-row">
        <div class="kpi"><span>在线坐席</span><strong>${state.live?.agents_online ?? 0}</strong></div>
        <div class="kpi"><span>排队</span><strong>${actions.waiting()}</strong></div>
        <div class="kpi"><span>今日接通</span><strong>${state.hist?.answered ?? state.cdr.filter((c) => c.result === "answered").length}</strong></div>
        <div class="kpi"><span>SIP</span><strong>${sipOk ? "已监听" : "关闭"}</strong></div>
      </div>
      <div class="split">
        ${renderPanel("实时队列", renderDataTable([
          { label: "队列", key: "name" },
          { label: "等待", key: "waiting" },
          { label: "空闲", render: () => actions.idleAgents() },
          { label: "策略", render: (q) => strategyLabel(state.queues.find((x) => x.name === q.name)?.strategy) },
          { label: "操作", render: () => html`<button class="ghost" @click=${() => actions.navigate("queues")}>查看</button>` },
        ], state.live?.queues || [], { emptyMessage: "暂无队列数据", label: "实时队列" }))}
        ${renderPanel("坐席状态", renderDataTable([
          { label: "坐席", render: (a) => a.display_name || a.extension },
          { label: "分机", render: (a) => a.extension || "" },
          { label: "状态", render: (a) => renderStatusTag(agentStateLabel(a.state), agentStateTone(a.state)) },
        ], state.agents, { emptyMessage: "暂无坐席", label: "坐席状态" }))}
      </div>
      ${renderPanel("话单", html`
        ${renderCdrQuery(state, actions)}
        ${renderCdrTable(state, actions, actions.filteredCdr().slice(0, 12))}
      `)}
    `;
  }
