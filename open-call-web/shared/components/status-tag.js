// 本文件负责状态标签组件。
import { html } from "lit";

export function renderStatusTag(label, tone = "muted") {
  return html`<span class="tag ${tone}">${label}</span>`;
}
