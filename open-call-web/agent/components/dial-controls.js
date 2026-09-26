// 本文件负责坐席拨号控制组件。
import { html } from "lit";
import { renderField } from "../../shared/components/panel.js";

export function renderDialControls(value, onChange, onDial) {
  return html`
    ${renderField("目标号码", html`<input .value=${value} @input=${(event) => onChange(event.target.value)} placeholder="bob / 分机 / 号码" />`)}
    <button @click=${onDial}>拨出</button>
  `;
}
