// 本文件负责文件选择控件。
import { html } from "lit";

export function renderFileInput({
  accept = "",
  disabled = false,
  onChange,
  buttonText = "选择文件",
  hint = "未选择文件",
} = {}) {
  return html`<label class="file${disabled ? " is-disabled" : ""}">
    <input type="file" accept=${accept} ?disabled=${!!disabled}
      @change=${(event) => {
        const name = event.target.files?.[0]?.name || "";
        const root = event.currentTarget.closest(".file");
        const label = root?.querySelector(".file-name");
        if (root) root.classList.toggle("has-file", !!name);
        if (label) label.textContent = name || hint;
        onChange?.(event);
      }} />
    <span class="file-btn">${buttonText}</span>
    <span class="file-name">${hint}</span>
  </label>`;
}
