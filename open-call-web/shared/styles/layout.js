// 本文件负责页面布局样式。
import { css } from "lit";

export const layoutStyles = css`
  .layout {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
  }

  .topbar {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-shrink: 0;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    font-weight: 600;
    white-space: nowrap;
  }

  .brand-mark {
    display: grid;
    place-items: center;
    font-size: 12px;
    font-weight: 700;
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
    flex-shrink: 0;
  }

  .nav-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    border: 0;
    background: transparent;
    text-align: left;
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
  }

  .breadcrumb strong {
    font-weight: 500;
  }

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

  @media (max-width: 960px) {
    .layout-body {
      flex-direction: column;
    }

    .topbar {
      padding: 0 14px;
    }

    .sidebar {
      display: flex;
      overflow-x: auto;
      width: 100%;
      padding: 8px 12px;
      border-right: 0;
      border-bottom: 1px solid var(--ov-border);
      scroll-snap-type: x proximity;
    }

    .nav-item {
      flex: 0 0 auto;
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
  }
`;
