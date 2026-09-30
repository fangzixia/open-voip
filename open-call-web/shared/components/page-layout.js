// 管理端统一页面模板：PageCrud / PageSplit / PageDashboard。
import { html, nothing } from "lit";
import { renderPanel } from "./panel.js";

/** 页壳：说明文案 + 内容区。 */
export function renderPage({ description = "", children = nothing } = {}) {
  return html`<div class="page">
    ${description ? html`<p class="page-desc">${description}</p>` : nothing}
    <div class="page-body">${children}</div>
  </div>`;
}

/**
 * CRUD 区块：说明 → 表单 → 列表（顺序固定）。
 * @param {{ title: string, hint?: string, form?: unknown, list?: unknown, footer?: unknown, className?: string }} section
 */
export function renderCrudSection({ title, hint = "", form = null, list = null, footer = null, className = "" } = {}) {
  return renderPanel(title, html`
    ${hint ? html`<p class="hint">${hint}</p>` : nothing}
    ${form ? html`<div class="page-form">${form}</div>` : nothing}
    ${list ? html`<div class="page-list">${list}</div>` : nothing}
    ${footer || nothing}
  `, { className });
}

/**
 * 标准 CRUD 页：页说明 + 多个 CRUD 区块。
 * @param {{ description?: string, sections: object[] }} opts
 */
export function renderPageCrud({ description = "", sections = [] } = {}) {
  return renderPage({
    description,
    children: html`<div class="page-sections">${sections.map((section) => renderCrudSection(section))}</div>`,
  });
}

/**
 * 主从页：左列表 / 右详情。
 * @param {{ description?: string, master: unknown, detail: unknown, below?: unknown }} opts
 */
export function renderPageSplit({ description = "", master = nothing, detail = nothing, below = nothing } = {}) {
  return renderPage({
    description,
    children: html`
      <div class="page-split">
        <div class="page-split-master">${master}</div>
        <div class="page-split-detail">${detail}</div>
      </div>
      ${below ? html`<div class="page-sections page-split-below">${below}</div>` : nothing}
    `,
  });
}

/**
 * 总览页：KPI + 卡片区 + 可选底部。
 * @param {{ description?: string, kpis?: { label: string, value: unknown }[], cards?: unknown[], footer?: unknown }} opts
 */
export function renderPageDashboard({ description = "", kpis = [], cards = [], footer = null } = {}) {
  return renderPage({
    description,
    children: html`
      ${kpis.length ? html`<div class="kpi-row">${kpis.map((k) => html`
        <div class="kpi"><span>${k.label}</span><strong>${k.value}</strong></div>
      `)}</div>` : nothing}
      ${cards.length ? html`<div class="page-dash-cards">${cards}</div>` : nothing}
      ${footer || nothing}
    `,
  });
}
