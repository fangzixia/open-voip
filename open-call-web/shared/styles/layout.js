import { css } from "lit";

export const layoutStyles = css`
  .topbar {
    height: 56px;
    padding: env(safe-area-inset-top, 0) var(--ov-space-6) 0;
    color: var(--ov-text);
    background: rgba(255, 255, 255, 0.96);
    border-bottom: 1px solid var(--ov-border);
    box-shadow: 0 1px 8px rgba(34, 42, 70, 0.03);
    position: sticky;
    top: 0;
    z-index: 20;
  }

  .brand {
    color: #263a78;
    font-size: 16px;
    letter-spacing: -0.01em;
  }

  .brand-mark {
    width: 30px;
    height: 30px;
    color: #fff;
    background: linear-gradient(135deg, #6366d9, #36b9d2);
    border-radius: var(--ov-radius-md);
    box-shadow: 0 4px 12px rgba(91, 91, 214, 0.2);
  }

  .brand-sub,
  .topbar-meta {
    color: var(--ov-text-secondary);
  }

  .topbar button:not(.nav-item) {
    color: var(--ov-text-secondary);
    background: #fff;
    border-color: var(--ov-border-strong);
    border-radius: var(--ov-radius-sm);
  }

  .topbar button:not(.nav-item):hover:not(:disabled) {
    color: var(--ov-primary);
    background: var(--ov-primary-soft);
    border-color: #c9c7f4;
  }

  .layout-body {
    background: var(--ov-bg);
  }

  .sidebar {
    width: 188px;
    padding: 18px 12px 28px;
    background: var(--ov-surface);
    border-right: 1px solid var(--ov-border);
  }

  .nav-item {
    position: relative;
    height: 40px;
    margin: 2px 0;
    padding: 0 14px;
    color: var(--ov-text-secondary);
    border-radius: var(--ov-radius-md);
    font-size: 13px;
    font-weight: 500;
  }

  .nav-section {
    display: block;
    padding: 12px 14px 5px;
    color: var(--ov-text-muted);
    font-size: 11px;
    letter-spacing: 0.06em;
  }

  .nav-item:hover {
    color: var(--ov-primary);
    background: #f8f7ff;
  }

  .nav-item.active {
    color: var(--ov-primary);
    background: var(--ov-primary-soft);
  }

  .nav-item.active::before {
    content: "";
    position: absolute;
    left: 0;
    width: 3px;
    height: 22px;
    border-radius: 0 3px 3px 0;
    background: var(--ov-accent);
  }

  .content {
    padding: var(--ov-space-5);
    padding-bottom: calc(var(--ov-space-5) + env(safe-area-inset-bottom, 0));
    background: var(--ov-bg);
  }

  .breadcrumb {
    margin-bottom: var(--ov-space-4);
    color: var(--ov-text-muted);
    font-size: 12px;
  }

  .breadcrumb strong {
    color: var(--ov-text-secondary);
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

  @media (max-width: 960px) {
    .topbar {
      padding: 0 14px;
    }

    .sidebar {
      width: 100%;
      padding: 8px 12px;
      border-right: 0;
      border-bottom: 1px solid var(--ov-border);
      scroll-snap-type: x proximity;
    }

    .nav-item {
      scroll-snap-align: start;
    }

    .nav-section {
      display: none;
    }
  }

  @media (max-width: 640px) {
    .topbar {
      height: auto;
      min-height: 56px;
      flex-wrap: wrap;
      padding-block: 10px;
    }

    .brand-sub,
    .topbar-meta:first-of-type {
      display: none;
    }

    .content {
      padding: 12px;
    }

    .panel {
      padding: 14px;
    }

    .form-inline .field {
      flex: 1 1 100%;
      min-width: 0;
    }
  }
`;
