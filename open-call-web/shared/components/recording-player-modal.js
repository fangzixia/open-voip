// 屏幕居中播放：标准浏览器 audio/video 控件。
import { html, nothing } from "lit";

export function renderRecordingPlayerModal({
  open = false,
  loading = false,
  error = "",
  playUrl = "",
  playType = "",
  onClose,
  onRetry,
  onBackdropClick,
}) {
  if (!open) return nothing;

  const stop = (e) => e.stopPropagation();

  return html`
    <div
      class="recording-modal"
      role="dialog"
      aria-modal="true"
      aria-label="播放录音"
      @keydown=${(e) => { if (e.key === "Escape") onClose?.(); }}
    >
      <button type="button" class="recording-modal__backdrop" aria-label="关闭" @click=${onBackdropClick || onClose}></button>
      <div class="recording-modal__panel" @click=${stop}>
        ${loading ? html`<p class="muted">正在加载录音…</p>` : nothing}
        ${error
          ? html`<p class="error" role="alert">${error}</p>
                 ${onRetry ? html`<button type="button" @click=${onRetry}>重试</button>` : nothing}`
          : nothing}
        ${!loading && !error && playUrl
          ? (playType.startsWith("video/")
            ? html`<video class="page-media recording-modal__media" controls playsinline src=${playUrl}></video>`
            : html`<audio class="page-media recording-modal__media" controls src=${playUrl}></audio>`)
          : nothing}
        <div class="recording-modal__actions">
          <button type="button" class="secondary" @click=${onClose}>关闭</button>
        </div>
      </div>
    </div>
  `;
}
