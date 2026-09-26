// 本文件负责录音管理页，展示文件和质检信息。
import { html } from "lit";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderPanel } from "../../shared/components/panel.js";
export function renderRecs(state, actions) {
    return renderPanel("录音", html`
        ${renderDataTable([
          { label: "ID", key: "id" },
          { label: "通话", key: "call_id" },
          { label: "类型", render: (r) => r.media_type === "video_composite"
              ? ["mp4", "webm"].includes(r.format) ? `视频 · ${r.format.toUpperCase()}` : "旧版音视频分离"
              : `音频 · ${(r.format || "ogg").toUpperCase()}` },
          { label: "大小", key: "file_size" },
          { label: "操作", render: (r) => state.me?.permissions?.includes("recordings.download") ? html`
              ${r.media_type === "video_composite" && ["mp4", "webm"].includes(r.format)
                ? html`<button class="secondary" @click=${() => actions.downloadRec(r.id, "mp4")}>下载 MP4</button>
                       <button class="secondary" @click=${() => actions.downloadRec(r.id, "webm")}>下载 WebM</button>`
                : html`<button class="secondary" @click=${() => actions.downloadRec(r.id)}>下载</button>`}
              <button class="secondary" @click=${() => actions.playRec(r.id)}>回放</button>
            ` : "" },
        ], state.recs, { showCount: true, label: "录音列表" })}
        ${state.playUrl
          ? state.playType.startsWith("video/")
            ? html`<video controls autoplay playsinline src=${state.playUrl}></video>`
            : html`<audio controls autoplay src=${state.playUrl}></audio>`
          : ""}
    `);
  }
