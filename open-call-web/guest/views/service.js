// 本文件负责访客服务详情视图。
import { html } from "lit";
import { renderServiceCard } from "../components/service-card.js";
export function renderInviteView(state, actions) {
    const video = state.inviteMedia === "video";
    return html`
      <div class="panel h5-invite">
        <div class="h5-invite-icon" aria-hidden="true">${video ? "▣" : "◉"}</div>
        <h2>${video ? "视频服务邀请" : "语音服务邀请"}</h2>
        <p class="muted">点击下方按钮后，浏览器将申请${video ? "摄像头和麦克风" : "麦克风"}权限，并进入服务队列。</p>
        <ul class="h5-checklist">
          <li>请使用稳定网络并保持页面开启</li>
          <li>通话可能因服务质量需要被录音</li>
          <li>邀请链接仅在有效期内可使用</li>
        </ul>
        <button class="h5-start" @click=${() => actions.startToken()}>${video ? "开始视频通话" : "开始语音通话"}</button>
        <p class="hint">继续即表示您同意使用设备权限完成本次服务。</p>
      </div>
    `;
  }

export function renderPickView(state, actions) {
    const sel = state.selected;
    return html`
      <div class="panel">
        <h3 class="page-title">选择服务</h3>
        ${state.queues.some((q) => q.video_enabled) ? html`
          <label>
            用户标识
            <input
              type="text"
              maxlength="64"
              autocomplete="username"
              placeholder="发起视频时必填，例如：客户编号或账号"
              .value=${state.userId}
              @input=${(e) => actions.setUserId(e.target.value)}
            />
          </label>
          <p class="hint">该标识将作为本次视频通话的访客身份信息。</p>
        ` : ""}
        <div class="svc-grid">
          ${!state.queues.length ? html`<div class="empty-state">暂无可用服务，请稍后重试</div>` : ""}
          ${[...state.queues.filter((q) => !q.video_enabled), ...state.queues.filter((q) => q.video_enabled)]
            .map((q) => renderServiceCard(q, {
              selected: sel?.queue?.id === q.id && !!sel.video === !!q.video_enabled,
              onPick: (queue, video) => actions.pick(queue, video, false),
              onStart: (queue, video) => actions.start(queue, video),
            }))}
        </div>
        ${state.queues.some((q) => q.priority_enabled)
          ? html`<p class="hint">部分队列支持 VIP 优先。</p>
              ${state.queues
                .filter((q) => q.priority_enabled)
                .map((q) => html`<button class="secondary" @click=${() => actions.start(q, false, true)}>${q.name} · VIP 语音</button>`)}`
          : ""}
      </div>
    `;
  }
