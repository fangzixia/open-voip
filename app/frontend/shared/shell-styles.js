import { css } from "lit";

/** 三端共用的基础布局与排版 */
export const shellStyles = css`
  :host {
    display: block;
    font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
    color: #1a1a1a;
    line-height: 1.5;
  }
  header {
    padding: 1rem 1.25rem;
    border-bottom: 1px solid #e5e5e5;
    background: #fafafa;
  }
  main {
    padding: 1.25rem;
    max-width: 960px;
    margin: 0 auto;
  }
  .badge {
    display: inline-block;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    background: #eef2ff;
    color: #3730a3;
    font-size: 0.85rem;
  }
  .error {
    color: #b91c1c;
  }
`;
