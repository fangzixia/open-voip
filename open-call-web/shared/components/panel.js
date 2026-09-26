// 本文件负责面板与表单字段组件。
import { html } from "lit";

/** 三类前端共用的内容区块容器。 */
export function renderPanel(title, content, { className = "" } = {}) {
  return html`<section class="panel ${className}">
    <h3>${title}</h3>
    ${content}
  </section>`;
}

export function renderField(label, control, { className = "", htmlFor = "" } = {}) {
  return html`<div class="field ${className}">
    ${htmlFor
      ? html`<label for=${htmlFor}>${label}</label>${control}`
      : html`<label>${label}${control}</label>`}
  </div>`;
}
