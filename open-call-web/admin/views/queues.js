// 本文件负责队列管理页，组织队列和技能操作（语音 / 视频分栏）。
import { html, nothing } from "lit";
import { strategyLabel, overflowPolicyLabel } from "./labels.js";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderField } from "../../shared/components/panel.js";
import { renderCheckbox } from "../../shared/components/checkbox.js";
import { renderPageCrud } from "../../shared/components/page-layout.js";

function sameKindQueues(queues, video) {
  return (queues || []).filter((q) => !!q.video_enabled === !!video);
}

function queueForm(state, video) {
  return video ? state.newVideoQueue : state.newVoiceQueue;
}

function queueSection(state, actions, { video, createTitle, listTitle }) {
  const canWrite = state.me?.permissions?.includes("queues.write");
  const peers = sameKindQueues(state.queues, video);
  const rows = sameKindQueues(state.queues, video);
  const form = queueForm(state, video);
  const patch = (p) => actions.updateNewQueue(!!video, p);
  return {
    title: listTitle,
    hint: video
      ? "仅用于访客端 WebRTC 视频服务，不可作为电话 DID 呼入目标。"
      : "用于电话 DID / IVR 呼入与访客语音服务。",
    form: canWrite ? html`<form @submit=${(e) => actions.createQueue(e, !!video)}>
      <div class="form-inline">
        ${renderField("名称", html`<input .value=${form.name} @input=${(e) => patch({ name: e.target.value })} />`)}
        ${renderField("溢出策略", html`<select .value=${form.overflow_policy} @change=${(e) => patch({ overflow_policy: e.target.value })}>
          <option value="hangup">超时结束</option>
          <option value="queue">溢出到另一队列</option>
          <option value="voicemail">留言结束</option>
        </select>`)}
        ${renderField("溢出目标", html`<select .value=${form.overflow_queue_id} @change=${(e) => patch({ overflow_queue_id: e.target.value })}>
          <option value="">无</option>
          ${peers.map((q) => html`<option value=${q.id}>${q.name}</option>`)}
        </select>`)}
        <button type="submit">${createTitle}</button>
      </div>
      ${renderCheckbox("VIP 优先", form.priority_enabled, (checked) => patch({ priority_enabled: checked }))}
      ${renderCheckbox("监听提示客户", form.listen_announce, (checked) => patch({ listen_announce: checked }))}
    </form>` : null,
    list: renderDataTable([
      { label: "名称", key: "name" },
      { label: "VIP", render: (q) => q.priority_enabled ? "是" : "否" },
      { label: "策略", render: (q) => strategyLabel(q.strategy) },
      { label: "溢出", render: (q) => overflowPolicyLabel(q.overflow_policy) },
      { label: "操作", render: (q) => canWrite ? html`
        <button class="secondary" @click=${() => actions.bindAll(q.id)}>绑定全部坐席</button>
        <button class="secondary" @click=${() => actions.toggleVip(q)}>VIP</button>
      ` : "" },
    ], rows, { showCount: true, label: listTitle, emptyMessage: video ? "暂无视频队列" : "暂无语音队列" }),
  };
}

export function renderQueues(state, actions) {
  const sections = [
    queueSection(state, actions, { video: false, createTitle: "创建语音队列", listTitle: "语音队列" }),
    queueSection(state, actions, { video: true, createTitle: "创建视频队列", listTitle: "视频队列" }),
  ];
  if (state.me?.permissions?.includes("skills.read")) {
    sections.push({
      title: "技能（语音 / 视频共用）",
      form: html`<div class="form-inline">
        ${renderField("技能名", html`<input .value=${state.skillName} @input=${(e) => actions.setSkillName(e.target.value)} />`)}
        ${state.me?.permissions?.includes("skills.write") ? html`<button type="button" class="secondary" @click=${() => actions.addSkill()}>创建技能</button>` : nothing}
        ${renderField("坐席", html`<select @change=${(e) => actions.setBindAgentId(e.target.value)}>
          <option value="">选择坐席</option>
          ${state.agents.map((a) => html`<option value=${a.id}>${a.display_name || a.extension}</option>`)}
        </select>`)}
        ${renderField("技能", html`<select @change=${(e) => actions.setBindSkillId(e.target.value)}>
          <option value="">选择技能</option>
          ${state.skills.map((s) => html`<option value=${s.id}>${s.name}</option>`)}
        </select>`)}
        ${state.me?.permissions?.includes("skills.write") ? html`<button type="button" class="secondary" @click=${() => actions.bindSkill()}>绑定技能</button>` : nothing}
      </div>`,
      footer: html`<p class="muted">已有技能：${state.skills.map((s) => s.name).join("、") || "无"}</p>`,
    });
  }
  return renderPageCrud({
    description: "语音与视频分属不同服务入口：电话呼入只进语音队列；视频仅出现在访客端。",
    sections,
  });
}
