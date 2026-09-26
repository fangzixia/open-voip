// 本文件负责复选框组件。
import { html } from "lit";

export function renderCheckbox(label, checked, onChange) {
  return html`<label class="check"><input type="checkbox" .checked=${checked}
    @change=${(event) => onChange(event.target.checked)} />${label}</label>`;
}
