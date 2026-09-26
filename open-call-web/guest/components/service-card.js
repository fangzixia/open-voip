import { html } from "lit";

/** 音频和视频服务共用的队列卡片。 */
export function renderServiceCard(queue, { selected, onPick, onStart }) {
  const video = !!queue.video_enabled;
  return html`<div class="svc-card ${selected ? "selected" : ""}" role="button" tabindex="0"
    @keydown=${(event) => { if (event.key === "Enter" || event.key === " ") onPick(queue, video); }}
    @click=${() => onPick(queue, video)}>
    <h3>${queue.name}</h3>
    <p class="muted">${video ? "需摄像头" : "麦克风通话 · 平均等待约 30 秒"}</p>
    <button class=${video ? "secondary" : ""}
      @click=${(event) => { event.stopPropagation(); onStart(queue, video); }}>${video ? "开始视频" : "开始通话"}</button>
  </div>`;
}
