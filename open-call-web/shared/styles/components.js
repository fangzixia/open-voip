// 本文件负责通用组件样式。
import { css } from "lit";

export const componentStyles = css`
  .page-title {
    margin: 0;
    font-size: 16px;
    font-weight: 600;
  }

  .split {
    display: grid;
    grid-template-columns: 1.4fr 1fr;
    gap: 16px;
    align-items: start;
  }

  .panel {
    padding: var(--ov-space-5);
    margin-bottom: var(--ov-space-4);
    border: 1px solid var(--ov-border);
    border-radius: var(--ov-radius-lg);
    background: var(--ov-surface);
    box-shadow: var(--ov-shadow-sm);
  }

  .panel h3,
  .panel h4,
  .page-title {
    color: var(--ov-text);
    letter-spacing: -0.01em;
  }

  .panel h3::before,
  .panel h4::before {
    content: "";
    display: inline-block;
    width: 3px;
    height: 14px;
    margin-right: 8px;
    border-radius: 2px;
    vertical-align: -2px;
    background: var(--ov-primary);
  }

  .login-page {
    background:
      radial-gradient(circle at 20% 15%, rgba(91, 91, 214, 0.08), transparent 32%),
      radial-gradient(circle at 80% 80%, rgba(49, 185, 210, 0.07), transparent 30%),
      var(--ov-bg);
  }

  .login-card {
    width: min(390px, 100%);
    padding: 32px;
    border: 1px solid var(--ov-border);
    border-radius: var(--ov-radius-lg);
    box-shadow: var(--ov-shadow-md);
  }

  .panel h3,
  .panel h4 {
    margin: 0 0 12px;
    font-size: 14px;
    font-weight: 600;
  }

  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    margin-bottom: 16px;
  }

  .toolbar.tight {
    margin-bottom: 12px;
  }

  button:not(.nav-item) {
    margin: 0;
  }

  button.danger:hover:not(:disabled) {
    background: #ff7875;
    border-color: #ff7875;
  }

  button.ghost {
    height: auto;
    line-height: inherit;
    padding: 0 4px;
  }

  .topbar button:not(.nav-item) {
    height: 28px;
    padding: 0 12px;
    line-height: 26px;
  }

  label {
    display: block;
    margin: 0 0 4px;
    color: rgba(0, 0, 0, 0.85);
    font-weight: 500;
  }

  .field {
    margin-bottom: 12px;
  }

  input,
  select,
  textarea {
    width: 100%;
    max-width: 100%;
    padding: 4px 11px;
    border: 1px solid #d9d9d9;
  }

  textarea {
    min-height: 72px;
    padding: 8px 11px;
    resize: vertical;
  }

  input:focus,
  select:focus,
  textarea:focus {
    outline: none;
  }

  .check {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 400;
    margin: 6px 0;
  }

  .check input {
    width: auto;
    height: auto;
    margin: 0;
  }

  .row {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    align-items: center;
  }

  .form-inline {
    display: flex;
    flex-wrap: wrap;
    gap: 12px 16px;
    align-items: flex-end;
    margin-bottom: 12px;
  }

  .form-inline .field {
    margin: 0;
    min-width: 180px;
  }

  .error {
    border-width: 1px;
    border-style: solid;
    margin-bottom: 12px;
  }

  .notice {
    border-width: 1px;
    border-style: solid;
    margin-bottom: 12px;
  }

  .hint {
    font-size: 12px;
    margin: 4px 0 0;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
    border: 1px solid #f0f0f0;
  }

  th,
  td {
    text-align: left;
    border-bottom: 1px solid #f0f0f0;
  }

  th {
    font-weight: 600;
    white-space: nowrap;
  }

  .pager {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 8px;
    margin-top: 12px;
    color: rgba(0, 0, 0, 0.45);
    font-size: 13px;
  }

  .kpi-row {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    margin-bottom: 16px;
  }

  .kpi span {
    display: block;
    color: rgba(0, 0, 0, 0.45);
    font-size: 13px;
  }

  .kpi strong {
    display: block;
    font-weight: 600;
    color: #1890ff;
    line-height: 1.2;
  }

  .tag {
    display: inline-block;
    font-size: 12px;
    background: #fafafa;
    color: rgba(0, 0, 0, 0.65);
  }

  .tag.ok {
    border-color: #b7eb8f;
  }

  .tag.info {
    border-color: #91d5ff;
  }

  .tag.warn {
    border-color: #ffe58f;
  }

  .tag.bad {
    border-color: #ffa39e;
  }

  .login-page {
    min-height: 100vh;
  }

  .login-page .topbar {
    margin-bottom: 0;
  }

  .login-wrap {
    min-height: calc(100vh - 48px);
    display: grid;
    place-items: center;
    padding: 24px;
  }

  .login-card {
    background: #fff;
  }

  .login-card h2 {
    margin: 0 0 4px;
    font-size: 18px;
  }

  .login-card .hint {
    margin-bottom: 16px;
  }

  .login-card button {
    width: 100%;
    margin-top: 8px;
  }

  .svc-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
    gap: 16px;
    margin-bottom: 16px;
  }

  .svc-card {
    border: 1px solid #d9d9d9;
    padding: 20px 18px;
    cursor: pointer;
    background: #fff;
  }

  .svc-card.selected {
    border-color: #1890ff;
    box-shadow: 0 0 0 1px #1890ff;
  }

  .svc-card h3 {
    margin: 0 0 8px;
    font-size: 16px;
  }

  .desc {
    display: grid;
    grid-template-columns: 120px 1fr;
    border: 1px solid #f0f0f0;
  }

  .desc dt,
  .desc dd {
    margin: 0;
    padding: 10px 12px;
    border-bottom: 1px solid #f0f0f0;
  }

  .desc dt {
    background: #fafafa;
    color: rgba(0, 0, 0, 0.65);
  }

  pre {
    overflow: auto;
    background: #fafafa;
    border: 1px solid #f0f0f0;
    padding: 12px;
    font-size: 12px;
  }

  .panel {
    overflow-x: auto;
  }

  button:not(.nav-item) {
    min-height: var(--ov-control-height);
    height: var(--ov-control-height);
    padding: 0 16px;
    color: #fff;
    background: linear-gradient(135deg, #625bd8, #4c67db);
    border: 1px solid transparent;
    border-radius: var(--ov-radius-sm);
    line-height: 34px;
    box-shadow: 0 3px 10px rgba(91, 91, 214, 0.14);
    transition: border-color 0.16s, background 0.16s, box-shadow 0.16s, transform 0.16s;
  }

  button:not(.nav-item):hover:not(:disabled) {
    background: linear-gradient(135deg, #544dc8, #405bc9);
    border-color: transparent;
    box-shadow: 0 5px 14px rgba(91, 91, 214, 0.22);
  }

  button:not(.nav-item):active:not(:disabled) {
    transform: translateY(1px);
  }

  button:focus-visible,
  .topbar button:focus-visible {
    outline: 2px solid var(--ov-primary);
    outline-offset: 2px;
    box-shadow: var(--ov-focus);
  }

  button.secondary {
    color: var(--ov-text-secondary);
    background: #fff;
    border-color: var(--ov-border-strong);
    box-shadow: none;
  }

  button.secondary:hover:not(:disabled) {
    color: var(--ov-primary);
    background: var(--ov-primary-soft);
    border-color: #c9c7f4;
  }

  button.danger {
    color: #fff;
    background: var(--ov-danger);
    border-color: var(--ov-danger);
  }

  button.ghost {
    color: var(--ov-primary);
    background: transparent;
    border-color: transparent;
    box-shadow: none;
  }

  input,
  select,
  textarea {
    min-height: var(--ov-control-height);
    height: var(--ov-control-height);
    color: var(--ov-text);
    background: #fff;
    border-color: var(--ov-border-strong);
    border-radius: var(--ov-radius-sm);
    transition: border-color 0.16s, box-shadow 0.16s;
  }

  textarea {
    height: auto;
  }

  input:focus,
  select:focus,
  textarea:focus {
    border-color: #9895e5;
    box-shadow: var(--ov-focus);
  }

  table {
    min-width: 620px;
    color: var(--ov-text-secondary);
    border-color: var(--ov-border);
    border-radius: var(--ov-radius-md);
    overflow: hidden;
  }

  th,
  td {
    padding: 11px 12px;
    border-color: var(--ov-border);
  }

  th {
    color: var(--ov-text-secondary);
    background: var(--ov-surface-soft);
    font-size: 12px;
    letter-spacing: 0.01em;
  }

  tr:hover td {
    background: #fafaff;
  }

  .kpi-row {
    gap: var(--ov-space-3);
  }

  .kpi {
    position: relative;
    min-height: 104px;
    padding: 18px 20px;
    border: 0;
    border-radius: var(--ov-radius-md);
    box-shadow: var(--ov-shadow-sm);
    overflow: hidden;
  }

  .kpi:nth-child(4n + 1) {
    background: linear-gradient(135deg, #6c6bd7, #8a5bd1);
  }

  .kpi:nth-child(4n + 2) {
    background: linear-gradient(135deg, #db65b4, #ef6f91);
  }

  .kpi:nth-child(4n + 3) {
    background: linear-gradient(135deg, #42a9df, #36c7d4);
  }

  .kpi:nth-child(4n + 4) {
    background: linear-gradient(135deg, #3bcaa0, #64d28a);
  }

  .kpi span,
  .kpi strong {
    color: #fff;
  }

  .kpi span {
    opacity: 0.84;
  }

  .kpi strong {
    margin-top: 8px;
    font-size: 28px;
    font-variant-numeric: tabular-nums;
  }

  .tag {
    height: 24px;
    padding: 0 9px;
    border: 0;
    border-radius: 999px;
    line-height: 24px;
  }

  .tag.ok {
    color: #287e50;
    background: #eaf7ef;
  }

  .tag.info {
    color: #356da9;
    background: #ecf4fd;
  }

  .tag.warn {
    color: #9b6a24;
    background: #fff5e6;
  }

  .tag.bad {
    color: #ad3445;
    background: #fff0f2;
  }

  .svc-card {
    border-color: var(--ov-border);
    border-radius: var(--ov-radius-lg);
    box-shadow: var(--ov-shadow-sm);
    transition: transform 0.18s, box-shadow 0.18s, border-color 0.18s;
  }

  .svc-card:hover,
  .svc-card.selected {
    border-color: #b7b4ed;
    box-shadow: var(--ov-shadow-md);
    transform: translateY(-2px);
  }

  .empty-state {
    padding: 28px 12px;
    color: var(--ov-text-muted);
    text-align: center;
  }

  .login-error,
  .section-spaced,
  .toolbar-spaced {
    margin-top: var(--ov-space-4);
  }

  .field-wide {
    min-width: 320px;
  }

  .field-narrow {
    max-width: 240px;
  }

  .invite-link {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--ov-space-2);
    align-items: end;
    margin-top: var(--ov-space-4);
  }

  .invite-link label {
    grid-column: 1 / -1;
  }

  @media (max-width: 960px) {
    .split {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 640px) {
    .panel {
      padding: 14px;
    }

    .form-inline .field {
      flex: 1 1 100%;
      min-width: 0;
    }

    button:not(.nav-item) {
      min-height: 44px;
      height: 44px;
      line-height: 42px;
    }

    .kpi-row {
      grid-template-columns: 1fr 1fr;
    }

    .kpi {
      min-height: 92px;
      padding: 14px;
    }
  }

  @media (max-width: 380px) {
    .kpi-row {
      grid-template-columns: 1fr;
    }
  }
`;
