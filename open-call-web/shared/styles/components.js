import { css } from "lit";

export const componentStyles = css`
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

  .table-scroll {
    width: 100%;
    overflow-x: auto;
    border-radius: var(--ov-radius-md);
    -webkit-overflow-scrolling: touch;
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

  .kpi-row,
  .metrics {
    gap: var(--ov-space-3);
  }

  .kpi,
  .metric {
    position: relative;
    min-height: 104px;
    padding: 18px 20px;
    border: 0;
    border-radius: var(--ov-radius-md);
    box-shadow: var(--ov-shadow-sm);
    overflow: hidden;
  }

  .kpi:nth-child(4n + 1),
  .metric:nth-child(4n + 1) {
    background: linear-gradient(135deg, #6c6bd7, #8a5bd1);
  }

  .kpi:nth-child(4n + 2),
  .metric:nth-child(4n + 2) {
    background: linear-gradient(135deg, #db65b4, #ef6f91);
  }

  .kpi:nth-child(4n + 3),
  .metric:nth-child(4n + 3) {
    background: linear-gradient(135deg, #42a9df, #36c7d4);
  }

  .kpi:nth-child(4n + 4),
  .metric:nth-child(4n + 4) {
    background: linear-gradient(135deg, #3bcaa0, #64d28a);
  }

  .kpi span,
  .metric span,
  .kpi strong,
  .metric strong {
    color: #fff;
  }

  .kpi span,
  .metric span {
    opacity: 0.84;
  }

  .kpi strong,
  .metric strong {
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

  @media (max-width: 640px) {
    button:not(.nav-item) {
      min-height: 44px;
      height: 44px;
      line-height: 42px;
    }

    .kpi-row,
    .metrics {
      grid-template-columns: 1fr 1fr;
    }

    .kpi,
    .metric {
      min-height: 92px;
      padding: 14px;
    }
  }

  @media (max-width: 380px) {
    .kpi-row,
    .metrics {
      grid-template-columns: 1fr;
    }
  }
`;
