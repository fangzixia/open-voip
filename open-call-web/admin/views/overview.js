// 本文件负责管理概览页，汇总运行状态。
import { html } from "lit";
import { strategyLabel } from "./labels.js";
import { agentStateLabel, agentStateTone } from "../../shared/display.js";
import { renderCdrQuery, renderCdrTable } from "./cdr.js";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderCrudSection, renderPageDashboard } from "../../shared/components/page-layout.js";
import { renderStatusTag } from "../../shared/components/status-tag.js";

export function renderOverview(state, actions) {
  const sipOk = state.status?.sip_listening ?? state.status?.sip_ok ?? true;
  return renderPageDashboard({
    kpis: [
      { label: "在线坐席", value: state.live?.agents_online ?? 0 },
      { label: "排队", value: actions.waiting() },
      { label: "今日接通", value: state.hist?.answered ?? state.cdr.filter((c) => c.result === "answered").length },
      { label: "SIP", value: sipOk ? "已监听" : "关闭" },
    ],
    cards: [
      renderCrudSection({
        title: "实时队列",
        list: renderDataTable([
          { label: "队列", key: "name" },
          { label: "等待", key: "waiting" },
          { label: "空闲", render: () => actions.idleAgents() },
          { label: "策略", render: (q) => strategyLabel(state.queues.find((x) => x.name === q.name)?.strategy) },
          { label: "操作", render: () => html`<button class="ghost" @click=${() => actions.navigate("queues")}>查看</button>` },
        ], state.live?.queues || [], { emptyMessage: "暂无队列数据", label: "实时队列" }),
      }),
      renderCrudSection({
        title: "坐席状态",
        list: renderDataTable([
          { label: "坐席", render: (a) => a.display_name || a.extension },
          { label: "分机", render: (a) => a.extension || "" },
          { label: "状态", render: (a) => renderStatusTag(agentStateLabel(a.state), agentStateTone(a.state)) },
        ], state.agents, { emptyMessage: "暂无坐席", label: "坐席状态" }),
      }),
    ],
    footer: renderCrudSection({
      title: "近期话单",
      form: renderCdrQuery(state, actions),
      list: renderCdrTable(state, actions, actions.filteredCdr().slice(0, 12)),
    }),
  });
}
