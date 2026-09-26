// 本文件负责话单页，展示通话记录。
import { html } from "lit";
import { callResultLabel, sessionTypeLabel } from "../../shared/call-enums.js";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderField, renderPanel } from "../../shared/components/panel.js";
export function renderCdrQuery(state, actions) {
    return html`
      <div class="form-inline">
        ${renderField("主叫", html`<input .value=${state.cdrCaller} @input=${(e) => actions.setCdrCaller(e.target.value)} placeholder="主叫号码" />`)}
        ${renderField("结果", html`<select .value=${state.cdrResult} @change=${(e) => actions.setCdrResult(e.target.value)}>
            <option value="">全部</option>
            <option value="answered">已接通</option>
            <option value="abandoned">已放弃</option>
            <option value="no_answer">未接听</option>
            <option value="failed">失败</option>
            <option value="timeout">超时</option>
            <option value="queued">排队中</option>
          </select>`)}
        <button @click=${() => actions.search()}>查询</button>
        <button class="secondary" @click=${() => actions.resetCdr()}>重置</button>
        ${state.me?.permissions?.includes("cdr.export") ? html`<button class="secondary" @click=${() => actions.exportCdr()}>导出</button>` : ""}
      </div>
    `;
  }

export function renderCdrTable(state, actions, rows) {
    return renderDataTable([
      { label: "call_id", key: "call_id" },
      { label: "主叫", render: (c) => c.caller || "—" },
      { label: "结果", render: (c) => callResultLabel(c.result) },
      { label: "媒介", render: (c) => sessionTypeLabel(c.session_type) },
      { label: "开始", render: (c) => c.started_at || "" },
      { label: "时长", render: (c) => `${c.duration_sec ?? 0}s` },
    ], rows, { emptyMessage: "暂无话单", showCount: true, label: "通话记录" });
  }

export function renderCdrPage(state, actions) {
    const rows = actions.filteredCdr();
    return html`
      ${renderPanel("通话记录", html`
        ${renderCdrQuery(state, actions)}
        ${renderCdrTable(state, actions, rows)}
      `)}
      ${renderPanel("通话小结", renderDataTable([
        { label: "时间", key: "created_at" },
        { label: "call_id", key: "call_id" },
        { label: "坐席", key: "agent_id" },
        { label: "内容", key: "notes" },
      ], state.wrapUps, { label: "通话小结" }))}
      ${state.me?.permissions?.includes("recordings.qa") ? renderPanel("质检标记", html`
        <form @submit=${(e) => actions.qa(e)}>
          <div class="form-inline">
            ${renderField("call_id", html`<input .value=${state.qaCallId} @input=${(e) => actions.setQaCallId(e.target.value)} />`)}
            ${renderField("标签", html`<input .value=${state.qaLabel} @input=${(e) => actions.setQaLabel(e.target.value)} />`)}
            <button type="submit">打点</button>
          </div>
        </form>
      `) : ""}
    `;
  }
