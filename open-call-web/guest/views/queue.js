// 本文件负责访客排队状态视图。
import { html } from "lit";
import { renderDialpad } from "../../shared/components/ui.js";
import { renderPanel } from "../../shared/components/panel.js";
import { renderDescriptionList } from "../../shared/components/description-list.js";
import { formatDuration } from "../../shared/display.js";
export function renderQueueIdleView(state, actions) {
    return renderPanel("排队状态", renderDescriptionList([
      ["前方等候", "0 位"],
      ["已等待", "00:00"],
      ["录音提示", "本通话可能会被录音"],
    ]));
  }

export function renderWaitView(state, actions) {
    return renderPanel("排队状态", html`
        <audio id="queue-audio" autoplay playsinline></audio>
        ${state.audioPlaybackBlocked ? html`<button @click=${() => actions.playRemoteAudio()}>播放声音</button>` : ""}
        <p class="muted">${state.permissionHint || "正在等待坐席接听…"}</p>
        <div class="queue-position" aria-label="前方等候 ${state.position || 0} 位">
          <div class="queue-position-inner">
            <strong>${state.position || 0}</strong>
            <span>前方等候</span>
          </div>
        </div>
        ${renderDescriptionList([
          ["前方等候", `${state.position || 0} 位`],
          ["已等待", formatDuration(state.waitSec)],
          ["录音提示", state.notice || "本通话可能会被录音"],
        ])}
        <p class="ivr-pad-label">IVR 请按键：</p>
        ${renderDialpad((digit) => actions.dtmf(digit))}
        <div class="toolbar toolbar-spaced">
          <button class="secondary" @click=${() => actions.hangup()}>结束排队</button>
        </div>
    `, { className: "queue-wait" });
  }
