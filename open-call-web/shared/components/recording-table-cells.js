// 话单/录音表四列：播放、格式、大小 KB、时长。
import { html, nothing } from "lit";
import {
  recordingDurationLabel,
  recordingFormatLabel,
  recordingPlayDisabled,
  recordingPlayDisabledTitle,
  recordingSizeKbLabel,
} from "../recording-meta.js";

const EMPTY = html`<span class="muted">—</span>`;

function canDownload(state) {
  return !!state.me?.permissions?.includes("recordings.download");
}

function renderPlayCell(r, actions, canDl) {
  if (!canDl) return EMPTY;
  const disabled = recordingPlayDisabled(r);
  const title = recordingPlayDisabledTitle(r);
  const video = r.media_type === "video_composite" && ["mp4", "webm"].includes((r.format || "").toLowerCase());
  return html`<span class="recording-play-actions">
    <button
      type="button"
      class="secondary"
      ?disabled=${disabled}
      title=${title || nothing}
      @click=${() => actions.playRec(r.id)}
    >播放</button>
    ${video
      ? html`<button type="button" class="ghost recording-dl" @click=${() => actions.downloadRec(r.id, "mp4")}>MP4</button>
             <button type="button" class="ghost recording-dl" @click=${() => actions.downloadRec(r.id, "webm")}>WebM</button>`
      : html`<button type="button" class="ghost recording-dl" @click=${() => actions.downloadRec(r.id)}>下载</button>`}
  </span>`;
}

function stackRow(recs, renderOne) {
  if (!recs?.length) return EMPTY;
  return html`<div class="recording-stack">
    ${recs.map((r) => html`<div class="recording-stack__row">${renderOne(r)}</div>`)}
  </div>`;
}

export function renderRecordingColumnStack(recs, column, state, actions) {
  const canDl = canDownload(state);
  switch (column) {
    case "play":
      return stackRow(recs, (r) => renderPlayCell(r, actions, canDl));
    case "format":
      return stackRow(recs, (r) => html`${recordingFormatLabel(r)}`);
    case "size":
      return stackRow(recs, (r) => html`${recordingSizeKbLabel(r)}`);
    case "duration":
      return stackRow(recs, (r) => html`${recordingDurationLabel(r)}`);
    default:
      return EMPTY;
  }
}

/** 话单表：一行通话对应多段录音的四列定义。 */
export function recordingCdrColumns(state, actions) {
  const col = (name, column) => ({
    label: name,
    render: (c) => renderRecordingColumnStack(c.recordings || [], column, state, actions),
  });
  return [
    col("录音", "play"),
    col("录音格式", "format"),
    col("录音大小", "size"),
    col("录音时长", "duration"),
  ];
}

/** 独立录音表：单行即一条录音的四列定义。 */
export function recordingEntityColumns(state, actions) {
  const col = (name, column) => ({
    label: name,
    render: (r) => renderRecordingColumnStack([r], column, state, actions),
  });
  return [
    col("录音", "play"),
    col("录音格式", "format"),
    col("录音大小", "size"),
    col("录音时长", "duration"),
  ];
}
