// 本文件负责访客通话视图。
import { html } from "lit";
import { renderDialpad } from "../../shared/components/ui.js";
import { formatDuration } from "../../shared/display.js";
import { renderPanel } from "../../shared/components/panel.js";
export function renderTalkView(state, actions) {
    return renderPanel(`通话中 ${formatDuration(state.elapsed)}`, html`
        ${state.notice ? html`<p class="notice" role="status" aria-live="polite">${state.notice}</p>` : ""}
        ${state.videoAsk
          ? html`<p class="notice" role="status" aria-live="polite">坐席请求开启视频
              <button @click=${() => actions.respondVideo(true)}>同意</button>
              <button class="secondary" @click=${() => actions.respondVideo(false)}>拒绝</button>
            </p>`
          : ""}
        <div class="row">
          <video id="remote" autoplay muted playsinline aria-label="坐席画面"></video>
          <audio id="remote-audio" autoplay playsinline></audio>
          <video id="local" autoplay muted playsinline aria-label="本地预览"></video>
        </div>
        ${state.audioPlaybackBlocked ? html`<button @click=${() => actions.playRemoteAudio()}>播放声音</button>` : ""}
        <div class="toolbar">
          <button class="secondary" aria-pressed=${state.audioMuted} @click=${() => actions.toggle("audio")}>${state.audioMuted ? "取消静音" : "静音"}</button>
          ${state.wantVideo
            ? html`<button class="secondary" aria-pressed=${state.videoMuted} @click=${() => actions.toggle("video")}>${state.videoMuted ? "开摄像头" : "关摄像头"}</button>`
            : html`<button class="secondary" @click=${() => actions.respondVideo(true)}>同意升视频</button>`}
          ${state.wantVideo ? html`<button class="secondary" @click=${() => actions.switchCamera()}>切换摄像头</button>` : ""}
          <button class="danger" @click=${() => actions.hangup()}>挂断</button>
        </div>
        ${renderDialpad((digit) => actions.dtmf(digit))}
    `, { className: "guest-talk" });
  }

export function renderEndedView(state, actions) {
    return renderPanel("通话已结束", html`
        <p class="muted">设备已释放。</p>
        <button @click=${() => actions.restart()}>返回</button>
    `);
  }
