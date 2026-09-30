// 本文件负责面板与表单字段组件。
import { html, nothing } from "lit";

/** 三类前端共用的内容区块容器。 */
export function renderPanel(title, content, { className = "" } = {}) {
  return html`<section class="panel ${className}">
    <h3>${title}</h3>
    ${content}
  </section>`;
}

/** 标签在上、控件在下；勿把 input 包进 label，避免行内表单错位。 */
export function renderField(label, control, { className = "", htmlFor = "", hint = "" } = {}) {
  return html`<div class="field ${className}">
    ${htmlFor
      ? html`<label class="field-label" for=${htmlFor}>${label}</label>`
      : html`<span class="field-label">${label}</span>`}
    ${control}
    ${hint ? html`<span class="hint field-hint">${hint}</span>` : nothing}
  </div>`;
}
