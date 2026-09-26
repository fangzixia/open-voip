// 本文件负责详情说明列表组件。
import { html } from "lit";

export function renderDescriptionList(items) {
  return html`<dl class="desc">
    ${items.map(([term, detail]) => html`<dt>${term}</dt><dd>${detail}</dd>`)}
  </dl>`;
}
