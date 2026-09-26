// 本文件负责坐席工作台，展示当前通话与操作入口。
import { html } from "lit";
import { renderDialpad } from "../../shared/components/ui.js";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderPanel } from "../../shared/components/panel.js";
import { renderDialControls } from "../components/dial-controls.js";
import { sessionTypeLabel } from "../../shared/call-enums.js";
import { formatDuration } from "../../shared/display.js";
export function renderDeskView(model, actions, state) {
    const inCall = !!model.call;
    const can = (code) => model.permissions?.includes(code);
    return html`
      <div class="panel">
        ${can("agents.self") ? html`<div class="toolbar tight">
          <button @click=${() => actions.doCheckIn()}>签入</button>
          <button class="secondary" @click=${() => actions.doCheckOut()}>签出</button>
          <button class="${state === "idle" ? "" : "secondary"}" @click=${() => actions.setIdle()}>示闲</button>
          <button class="${state === "busy" ? "" : "secondary"}" @click=${() => actions.toggleBusy()}>示忙</button>
        </div>` : ""}
        ${can("calls.operate") && model.incoming && !inCall ? renderIncomingBar(model, actions) : ""}
        ${can("calls.wrap_up") && !inCall && model.pendingWrapId ? renderAcwPanel(model, actions) : ""}
        <div class="workbench">
          ${renderStagePanel(model, actions, inCall)}
          ${renderSideCard(model, actions)}
        </div>
        <h4>近期通话</h4>
        ${renderDataTable([
          { label: "时间", key: "time" },
          { label: "主叫", key: "caller" },
          { label: "队列", key: "queue" },
          { label: "结果", key: "result" },
          { label: "时长", render: (c) => formatDuration(c.duration) },
        ], model.recentCalls, { emptyMessage: "暂无本会话通话记录", showCount: true, label: "近期通话" })}
      </div>
    `;
  }

export function renderIncomingBar(model, actions) {
    return html`
      <div class="incoming-bar">
        <strong>来电</strong>
        <span>${model.incoming.caller || "未知主叫"}</span>
        <span class="muted">${model.incoming.queue_name || "—"} · ${sessionTypeLabel(model.incoming.session_type)}</span>
        <span class="spacer"></span>
        <button @click=${() => actions.answer()}>接听</button>
        <button class="danger" @click=${() => actions.decline()}>拒接</button>
      </div>
    `;
  }

export function renderAcwPanel(model, actions) {
    return html`
      <div class="incoming-bar">
        <strong>事后处理</strong>
        <span class="muted">通话 ${model.pendingWrapId}</span>
        <textarea class="acw-notes" .value=${model.wrapNotes} @input=${(e) => actions.setWrapNotes(e.target.value)} placeholder="填写通话小结"></textarea>
        <button @click=${() => actions.submitWrap()}>提交小结并示闲</button>
      </div>
    `;
  }

export function renderStagePanel(model, actions, inCall) {
    return html`
      <div class="stage ${model.sharing ? "share" : ""}">
        <div class="stage-timer">${formatDuration(model.elapsed)}</div>
        <div class="stage-meta">
          ${actions.stageCaller()}
          ${actions.stageQueue() !== "—" ? html` · ${actions.stageQueue()}` : ""}
          ${model.call?.session_type ? html` · ${sessionTypeLabel(model.call.session_type)}` : ""}
          ${model.held ? html` · 保持` : ""}
          ${!inCall ? html` · 空闲` : ""}
        </div>
        ${model.permissions?.includes("calls.operate") && model.videoAsk && inCall
          ? html`<p class="notice">对端请求升视频
              <button @click=${() => actions.respondVideo(true)}>同意</button>
              <button class="secondary" @click=${() => actions.respondVideo(false)}>拒绝</button>
            </p>`
          : ""}
        ${inCall && model.cameraUnavailable
          ? html`<p class="notice">本机没有可用摄像头，已用麦克风接通；仍可观看访客视频。</p>`
          : ""}
        ${inCall && model.audioPlaybackBlocked
          ? html`<p class="notice">浏览器已暂停通话声音 <button @click=${() => actions.playRemoteAudio()}>播放声音</button></p>`
          : ""}
        ${inCall || model.hasLocal
          ? html`<div class="stage-videos">
              ${inCall ? html`<video id="remote" autoplay muted playsinline></video><audio id="remote-audio" autoplay playsinline></audio>` : ""}
              <video id="local" autoplay muted playsinline></video>
            </div>`
          : ""}
        ${model.permissions?.includes("calls.operate") ? html`<div class="stage-controls">
          <button class="ctl ${model.audioMuted ? "on" : ""}" ?disabled=${!inCall} @click=${() => actions.toggleMute("audio")}>${model.audioMuted ? "取消静音" : "静音"}</button>
          <button class="ctl ${model.held ? "on" : ""}" ?disabled=${!inCall} @click=${() => actions.hold()}>${model.held ? "恢复" : "保持"}</button>
          <button class="ctl ${model.showPad ? "on" : ""}" ?disabled=${!inCall} @click=${() => actions.setShowPad(!model.showPad)}>键盘</button>
          <button class="ctl" ?disabled=${!inCall} @click=${() => actions.xfer()}>转接</button>
          <button class="hangup" ?disabled=${!inCall} @click=${() => actions.hangup()}>挂断</button>
        </div>` : ""}
        ${model.permissions?.includes("calls.operate") && model.showPad && inCall ? renderDialpad((digit) => actions.dtmf(digit)) : ""}
        ${model.permissions?.includes("calls.operate") && inCall
          ? html`<div class="stage-extra">
              <button ?disabled=${model.cameraUnavailable} @click=${() => actions.toggleMute("video")}>${model.cameraUnavailable ? "无摄像头" : model.videoMuted ? "开摄像头" : "关摄像头"}</button>
              <button @click=${() => actions.share()}>屏幕共享</button>
              <button @click=${() => actions.askVideo()}>升视频</button>
              <button @click=${() => actions.downgrade()}>降为语音</button>
              <select .value=${model.xferMode} @change=${(e) => actions.setXferMode(e.target.value)}>
                <option value="blind">盲转</option>
                <option value="consult">咨询转</option>
              </select>
              ${model.consulting ? html`<button @click=${() => actions.completeXfer()}>完成转接</button>` : ""}
              <button @click=${() => actions.conf()}>邀请三方</button>
              ${model.permissions?.includes("calls.wrap_up") ? html`<button @click=${() => actions.submitWrap()}>提交小结</button>` : ""}
            </div>
            <textarea class="wrap-notes" .value=${model.wrapNotes} @input=${(e) => actions.setWrapNotes(e.target.value)} placeholder="通话小结"></textarea>`
          : ""}
      </div>
    `;
  }

export function renderSideCard(model, actions) {
    return html`
      <div>
        ${model.permissions?.includes("calls.operate") ? renderPanel("外呼", renderDialControls(model.dest, actions.setDest, actions.dial), { className: "side-panel" }) : ""}
        ${renderPanel("排队", html`
          <p class="queue-count">${actions.waitingCount()} <span class="queue-count-unit">人</span></p>
        `)}
      </div>
    `;
  }
