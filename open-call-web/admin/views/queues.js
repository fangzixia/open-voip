// 本文件负责队列管理页，组织队列和技能操作。
import { html } from "lit";
import { strategyLabel, overflowPolicyLabel } from "./labels.js";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderField, renderPanel } from "../../shared/components/panel.js";
import { renderCheckbox } from "../../shared/components/checkbox.js";
export function renderQueues(state, actions) {
    return html`
      ${state.me?.permissions?.includes("queues.write") ? renderPanel("新建队列", html`
        <form @submit=${(e) => actions.createQueue(e)}>
          <div class="form-inline">
            ${renderField("名称", html`<input .value=${state.newQueue.name} @input=${(e) => actions.updateNewQueue({ name: e.target.value })} />`)}
            ${renderField("溢出策略", html`<select .value=${state.newQueue.overflow_policy} @change=${(e) => actions.updateNewQueue({ overflow_policy: e.target.value })}>
                <option value="hangup">超时结束</option>
                <option value="queue">溢出到另一队列</option>
                <option value="voicemail">留言结束</option>
              </select>`)}
            ${renderField("溢出目标队列 ID", html`<input .value=${state.newQueue.overflow_queue_id} @input=${(e) => actions.updateNewQueue({ overflow_queue_id: e.target.value })} />`)}
            <button type="submit">创建队列</button>
          </div>
          ${renderCheckbox("视频队列", state.newQueue.video_enabled, (checked) => actions.updateNewQueue({ video_enabled: checked }))}
          ${renderCheckbox("VIP 优先", state.newQueue.priority_enabled, (checked) => actions.updateNewQueue({ priority_enabled: checked }))}
          ${renderCheckbox("监听提示客户", state.newQueue.listen_announce, (checked) => actions.updateNewQueue({ listen_announce: checked }))}
        </form>
      `) : ""}
      ${state.me?.permissions?.includes("skills.read") ? renderPanel("技能", html`
        <div class="form-inline">
          ${renderField("技能名", html`<input .value=${state.skillName} @input=${(e) => actions.setSkillName(e.target.value)} />`)}
          ${state.me?.permissions?.includes("skills.write") ? html`<button class="secondary" @click=${() => actions.addSkill()}>创建技能</button>` : ""}
          ${renderField("坐席", html`<select @change=${(e) => actions.setBindAgentId(e.target.value)}>
              <option value="">选择坐席</option>
              ${state.agents.map((a) => html`<option value=${a.id}>${a.display_name || a.extension}</option>`)}
            </select>`)}
          ${renderField("技能", html`<select @change=${(e) => actions.setBindSkillId(e.target.value)}>
              <option value="">选择技能</option>
              ${state.skills.map((s) => html`<option value=${s.id}>${s.name}</option>`)}
            </select>`)}
          ${state.me?.permissions?.includes("skills.write") ? html`<button class="secondary" @click=${() => actions.bindSkill()}>绑定技能</button>` : ""}
        </div>
        <p class="muted">已有技能：${state.skills.map((s) => s.name).join("、") || "无"}</p>
      `) : ""}
      ${renderPanel("队列列表", renderDataTable([
        { label: "名称", key: "name" },
        { label: "视频", render: (q) => q.video_enabled ? "是" : "否" },
        { label: "VIP", render: (q) => q.priority_enabled ? "是" : "否" },
        { label: "策略", render: (q) => strategyLabel(q.strategy) },
        { label: "溢出", render: (q) => overflowPolicyLabel(q.overflow_policy) },
        { label: "操作", render: (q) => state.me?.permissions?.includes("queues.write") ? html`
                <button class="secondary" @click=${() => actions.bindAll(q.id)}>绑定全部坐席</button>
                <button class="secondary" @click=${() => actions.toggleVip(q)}>VIP</button>
              ` : "" },
      ], state.queues, { showCount: true, label: "队列列表" }))}
    `;
  }
