import { html, nothing } from "lit";

/** 表格列包含标题，并通过字段名或渲染函数提供单元格内容。 */
export function renderDataTable(columns, rows, { emptyMessage = "", showCount = false, label = "" } = {}) {
  return html`
    <table aria-label=${label || nothing}>
      <thead><tr>${columns.map((column) => html`<th scope="col">${column.label}</th>`)}</tr></thead>
      <tbody>
        ${rows.length
          ? rows.map((row) => html`<tr>${columns.map((column) => html`<td>${column.render ? column.render(row) : row[column.key] ?? ""}</td>`)}</tr>`)
          : emptyMessage
            ? html`<tr><td colspan=${columns.length}><div class="empty-state">${emptyMessage}</div></td></tr>`
            : nothing}
      </tbody>
    </table>
    ${showCount ? html`<div class="pager">共 ${rows.length} 条</div>` : nothing}
  `;
}
