// 本文件负责录音管理页，展示文件和质检信息。
import { html } from "lit";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderPageCrud } from "../../shared/components/page-layout.js";

export function renderRecs(state, actions) {
  const canDownload = state.me?.permissions?.includes("recordings.download");
  return renderPageCrud({
    sections: [{
      title: "录音列表",
      list: renderDataTable([
        { label: "ID", key: "id" },
        { label: "通话", key: "call_id" },
        { label: "类型", render: (r) => r.media_type === "video_composite"
            ? ["mp4", "webm"].includes(r.format) ? `视频 · ${r.format.toUpperCase()}` : "旧版音视频分离"
            : `音频 · ${(r.format || "ogg").toUpperCase()}` },
        { label: "大小", key: "file_size" },
        { label: "录制内容", render: (r) => r.recording_semantics === "conversation_mono_v1" ? "完整通话（单声道）" : "历史录音" },
        { label: "时长", render: (r) => r.sample_rate_hz > 0 && r.duration_samples >= 0 ? `${(r.duration_samples / r.sample_rate_hz).toFixed(2)} 秒` : "—" },
        { label: "状态", render: (r) => r.status === "failed" ? `失败 · ${r.failure_reason || "录音不完整"}` : r.status === "recording" ? "录制中" : r.status === "completed" ? "已完成" : "状态未知" },
        { label: "操作", render: (r) => canDownload ? html`
            ${r.media_type === "video_composite" && ["mp4", "webm"].includes(r.format)
              ? html`<button class="secondary" @click=${() => actions.downloadRec(r.id, "mp4")}>下载 MP4</button>
                     <button class="secondary" @click=${() => actions.downloadRec(r.id, "webm")}>下载 WebM</button>`
              : html`<button class="secondary" @click=${() => actions.downloadRec(r.id)}>下载</button>`}
            <button class="secondary" @click=${() => actions.playRec(r.id)}>回放</button>
          ` : "" },
      ], state.recs, { showCount: true, label: "录音列表", emptyMessage: "暂无录音" }),
      footer: state.playUrl
        ? (state.playType.startsWith("video/")
          ? html`<video class="page-media" controls autoplay playsinline src=${state.playUrl}></video>`
          : html`<audio class="page-media" controls autoplay src=${state.playUrl}></audio>`)
        : null,
    }],
  });
}
