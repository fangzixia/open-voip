import { css } from "lit";

/** 传统后台壳层 + 坐席深色通话台 */
export const shellStyles = css`
  :host {
    display: block;
    min-height: 100vh;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB",
      "Microsoft YaHei", "Helvetica Neue", Helvetica, Arial, sans-serif;
    color: #262626;
    line-height: 1.5;
    background: #f0f2f5;
    font-size: 14px;
  }

  * {
    box-sizing: border-box;
  }

  button {
    font: inherit;
    cursor: pointer;
  }

  input,
  select,
  textarea {
    font: inherit;
  }

  .layout {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
  }

  .topbar {
    height: 48px;
    background: #001529;
    color: #fff;
    display: flex;
    align-items: center;
    padding: 0 16px;
    gap: 12px;
    flex-shrink: 0;
    z-index: 10;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    font-weight: 600;
    letter-spacing: 0.02em;
    white-space: nowrap;
  }

  .brand-mark {
    width: 28px;
    height: 28px;
    background: #fff;
    color: #001529;
    display: grid;
    place-items: center;
    font-size: 12px;
    font-weight: 700;
    border-radius: 2px;
  }

  .brand-sub {
    color: rgba(255, 255, 255, 0.65);
    font-weight: 400;
    font-size: 13px;
  }

  .spacer {
    flex: 1;
  }

  .topbar-meta {
    color: rgba(255, 255, 255, 0.85);
    font-size: 13px;
  }

  .topbar button {
    margin: 0;
    height: 28px;
    padding: 0 12px;
    border: 1px solid rgba(255, 255, 255, 0.25);
    background: transparent;
    color: #fff;
    border-radius: 2px;
  }

  .topbar button:hover {
    border-color: #1890ff;
    color: #1890ff;
  }

  .layout-body {
    flex: 1;
    display: flex;
    min-height: 0;
  }

  .sidebar {
    width: 200px;
    flex-shrink: 0;
    background: #001529;
    padding: 8px 0 24px;
  }

  .nav-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    height: 40px;
    margin: 0;
    padding: 0 16px 0 24px;
    border: 0;
    border-radius: 0;
    background: transparent;
    color: rgba(255, 255, 255, 0.65);
    text-align: left;
    font-size: 14px;
  }

  .nav-item:hover {
    color: #fff;
    background: rgba(255, 255, 255, 0.04);
  }

  .nav-item.active {
    color: #fff;
    background: #1890ff;
  }

  .nav-badge {
    min-width: 16px;
    height: 16px;
    padding: 0 5px;
    border-radius: 8px;
    background: #ff4d4f;
    color: #fff;
    font-size: 11px;
    line-height: 16px;
    text-align: center;
  }

  .nav-item.active .nav-badge {
    background: #fff;
    color: #1890ff;
  }

  .content {
    flex: 1;
    min-width: 0;
    padding: 16px;
    background: #f0f2f5;
  }

  .breadcrumb {
    color: rgba(0, 0, 0, 0.45);
    font-size: 13px;
    margin-bottom: 12px;
  }

  .breadcrumb strong {
    color: rgba(0, 0, 0, 0.85);
    font-weight: 500;
  }

  .page-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
  }

  .page-title {
    margin: 0;
    font-size: 16px;
    font-weight: 600;
  }

  .panel {
    background: #fff;
    border: 1px solid #d9d9d9;
    border-radius: 2px;
    padding: 16px;
    margin-bottom: 16px;
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
    height: 32px;
    padding: 0 15px;
    border: 1px solid #1890ff;
    border-radius: 2px;
    background: #1890ff;
    color: #fff;
    line-height: 30px;
  }

  button:not(.nav-item):hover:not(:disabled) {
    background: #40a9ff;
    border-color: #40a9ff;
  }

  button.secondary {
    background: #fff;
    color: rgba(0, 0, 0, 0.85);
    border-color: #d9d9d9;
  }

  button.secondary:hover:not(:disabled) {
    color: #1890ff;
    border-color: #1890ff;
    background: #fff;
  }

  button.danger {
    background: #ff4d4f;
    border-color: #ff4d4f;
    color: #fff;
  }

  button.danger:hover:not(:disabled) {
    background: #ff7875;
    border-color: #ff7875;
  }

  button.ghost {
    background: transparent;
    border-color: transparent;
    color: #1890ff;
    height: auto;
    line-height: inherit;
    padding: 0 4px;
  }

  button:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .topbar button:not(.nav-item) {
    height: 28px;
    padding: 0 12px;
    border: 1px solid rgba(255, 255, 255, 0.25);
    background: transparent;
    color: #fff;
    line-height: 26px;
  }

  .topbar button:not(.nav-item):hover:not(:disabled) {
    border-color: #40a9ff;
    color: #40a9ff;
    background: transparent;
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
    height: 32px;
    padding: 4px 11px;
    border: 1px solid #d9d9d9;
    border-radius: 2px;
    background: #fff;
    color: #262626;
  }

  textarea {
    height: auto;
    min-height: 72px;
    padding: 8px 11px;
    resize: vertical;
  }

  input:focus,
  select:focus,
  textarea:focus {
    outline: none;
    border-color: #1890ff;
    box-shadow: 0 0 0 2px rgba(24, 144, 255, 0.2);
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
    color: #cf1322;
    background: #fff1f0;
    border: 1px solid #ffa39e;
    padding: 8px 12px;
    margin-bottom: 12px;
    border-radius: 2px;
  }

  .notice {
    color: #096dd9;
    background: #e6f7ff;
    border: 1px solid #91d5ff;
    padding: 8px 12px;
    margin-bottom: 12px;
    border-radius: 2px;
  }

  .muted {
    color: rgba(0, 0, 0, 0.45);
  }

  .hint {
    color: rgba(0, 0, 0, 0.45);
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
    padding: 10px 12px;
    border-bottom: 1px solid #f0f0f0;
  }

  th {
    background: #fafafa;
    color: rgba(0, 0, 0, 0.85);
    font-weight: 600;
    white-space: nowrap;
  }

  tr:hover td {
    background: #fafafa;
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

  .metrics,
  .kpi-row {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: 12px;
    margin-bottom: 16px;
  }

  .metric,
  .kpi {
    background: #fff;
    border: 1px solid #d9d9d9;
    border-radius: 2px;
    padding: 16px 18px;
  }

  .metric span,
  .kpi span {
    display: block;
    color: rgba(0, 0, 0, 0.45);
    font-size: 13px;
  }

  .metric strong,
  .kpi strong {
    display: block;
    margin-top: 6px;
    font-size: 24px;
    font-weight: 600;
    color: #1890ff;
    line-height: 1.2;
  }

  .tag {
    display: inline-block;
    padding: 0 7px;
    height: 22px;
    line-height: 20px;
    font-size: 12px;
    border: 1px solid #d9d9d9;
    border-radius: 2px;
    background: #fafafa;
    color: rgba(0, 0, 0, 0.65);
  }

  .tag.ok {
    color: #52c41a;
    background: #f6ffed;
    border-color: #b7eb8f;
  }

  .tag.info {
    color: #1890ff;
    background: #e6f7ff;
    border-color: #91d5ff;
  }

  .tag.warn {
    color: #d48806;
    background: #fffbe6;
    border-color: #ffe58f;
  }

  .tag.bad {
    color: #cf1322;
    background: #fff1f0;
    border-color: #ffa39e;
  }

  .split {
    display: grid;
    grid-template-columns: 1.4fr 1fr;
    gap: 16px;
    align-items: start;
  }

  .workbench {
    display: grid;
    grid-template-columns: 1fr 280px;
    gap: 16px;
    align-items: start;
  }

  .incoming-bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    background: #fff;
    border: 1px solid #d9d9d9;
    border-left: 3px solid #1890ff;
    padding: 12px 16px;
    margin-bottom: 16px;
  }

  .incoming-bar strong {
    font-size: 15px;
  }

  .stage {
    background: #0f172a;
    border-radius: 12px;
    color: #e2e8f0;
    padding: 28px 20px 20px;
    text-align: center;
    min-height: 300px;
  }

  .stage-timer {
    font-size: 56px;
    font-weight: 600;
    letter-spacing: 3px;
    font-variant-numeric: tabular-nums;
    color: #fff;
    line-height: 1;
  }

  .stage-meta {
    margin: 12px 0 22px;
    color: #94a3b8;
    font-size: 14px;
  }

  .stage-videos {
    display: flex;
    justify-content: center;
    gap: 12px;
    margin-bottom: 18px;
  }

  .stage video {
    width: 100%;
    max-width: 420px;
    background: #020617;
    border-radius: 8px;
  }

  .stage-videos video#local {
    max-width: 160px;
  }

  .stage.share video#remote {
    max-width: 100%;
    flex: 2 1 520px;
    min-height: 240px;
  }

  video {
    width: 100%;
    max-width: 420px;
    background: #111;
    border-radius: 8px;
  }

  .stage-controls {
    display: flex;
    justify-content: center;
    align-items: center;
    gap: 14px;
    flex-wrap: wrap;
  }

  button.ctl {
    width: 56px;
    height: 56px;
    margin: 0;
    padding: 0;
    border-radius: 50%;
    background: #1e293b;
    border: 1px solid #334155;
    color: #e2e8f0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 2px;
    font-size: 11px;
    line-height: 1.1;
    box-shadow: none;
  }

  button.ctl:hover:not(:disabled) {
    background: #334155;
    border-color: #475569;
    color: #fff;
  }

  button.ctl.on {
    background: #1890ff;
    border-color: #1890ff;
    color: #fff;
  }

  button.hangup {
    width: 64px;
    height: 64px;
    margin: 0;
    padding: 0;
    border-radius: 50%;
    background: #ff4d4f;
    border-color: #ff4d4f;
    color: #fff;
    font-size: 12px;
  }

  button.hangup:hover:not(:disabled) {
    background: #ff7875;
    border-color: #ff7875;
  }

  .stage-extra {
    margin-top: 16px;
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 8px;
  }

  .stage-extra button {
    background: #1e293b;
    border-color: #334155;
    color: #e2e8f0;
  }

  .stage-extra button:hover:not(:disabled) {
    background: #334155;
    border-color: #475569;
    color: #fff;
  }

  .stage-extra select {
    width: auto;
    background: #1e293b;
    border-color: #334155;
    color: #e2e8f0;
  }

  .dialpad {
    display: grid;
    grid-template-columns: repeat(3, 48px);
    gap: 8px;
    justify-content: center;
    margin: 16px auto 0;
  }

  .dialpad button {
    width: 48px;
    height: 48px;
    margin: 0;
    padding: 0;
    border-radius: 50%;
    background: #1e293b;
    border-color: #334155;
    color: #fff;
  }

  .login-page {
    min-height: 100vh;
    background: #f0f2f5;
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
    width: 360px;
    background: #fff;
    border: 1px solid #d9d9d9;
    padding: 28px 24px 24px;
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
    border-radius: 2px;
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

  .guest-talk video {
    max-width: 100%;
  }

  @media (max-width: 960px) {
    .layout-body {
      flex-direction: column;
    }
    .sidebar {
      width: 100%;
      display: flex;
      overflow-x: auto;
      padding: 0;
    }
    .nav-item {
      flex: 0 0 auto;
      padding: 0 16px;
    }
    .workbench,
    .split {
      grid-template-columns: 1fr;
    }
    .stage-timer {
      font-size: 40px;
    }
  }
`;
