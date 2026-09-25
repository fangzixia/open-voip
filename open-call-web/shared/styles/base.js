import { css } from "lit";

export const baseStyles = css`
  :host {
    color: var(--ov-text);
    background: var(--ov-bg);
    font-family: Inter, "SF Pro Text", -apple-system, BlinkMacSystemFont, "Segoe UI",
      "PingFang SC", "Microsoft YaHei", sans-serif;
    font-size: 14px;
    line-height: 1.55;
    -webkit-font-smoothing: antialiased;
    text-rendering: optimizeLegibility;
  }

  button,
  input,
  select,
  textarea {
    font: inherit;
  }

  button:focus-visible,
  input:focus-visible,
  select:focus-visible,
  textarea:focus-visible,
  .svc-card:focus-visible {
    outline: none;
    box-shadow: var(--ov-focus);
  }

  button:disabled {
    cursor: not-allowed;
    opacity: 0.48;
  }

  .muted,
  .hint {
    color: var(--ov-text-muted);
  }

  .error,
  .notice {
    border-radius: var(--ov-radius-md);
    padding: 10px 14px;
  }

  .error {
    color: #ad3445;
    background: #fff5f6;
    border-color: #f4c7ce;
  }

  .notice {
    color: #315f9b;
    background: #f1f7ff;
    border-color: #c8ddf5;
  }
`;
