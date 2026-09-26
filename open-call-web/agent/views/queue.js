// 本文件负责坐席队列状态视图。
import { html } from "lit";
import { renderField, renderPanel } from "../../shared/components/panel.js";
import { renderCheckbox } from "../../shared/components/checkbox.js";
export function renderQueueView(model, actions, state) {
    return renderPanel("队列签入", html`
        ${model.queues.map((q) => renderCheckbox(
          `${q.name} ${q.video_enabled ? "(视频)" : "(语音)"}`,
          model.selectedQueues.includes(q.id),
          (checked) => actions.setSelectedQueues(checked
            ? [...model.selectedQueues, q.id]
            : model.selectedQueues.filter((id) => id !== q.id)),
        ))}
        <div class="toolbar">
          <button @click=${() => actions.doCheckIn()}>签入</button>
          <button class="secondary" @click=${() => actions.doCheckOut()}>签出</button>
          <button class="secondary" @click=${() => actions.toggleBusy()}>${state === "busy" ? "示闲" : "示忙"}</button>
        </div>
        ${renderField("示忙原因", html`<select .value=${model.busyReason} @change=${(e) => actions.setBusyReason(e.target.value)}>
            <option value="break">小休</option>
            <option value="training">培训</option>
            <option value="meeting">会议</option>
          </select>`, { className: "field-narrow" })}
    `);
  }
