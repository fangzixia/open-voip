import { css } from "lit";

export const callStyles = css`
  .workbench {
    grid-template-columns: minmax(0, 1fr) 300px;
  }

  .incoming-bar {
    border: 1px solid #d7d5f5;
    border-left: 4px solid var(--ov-primary);
    border-radius: var(--ov-radius-md);
    background: #fbfaff;
    box-shadow: var(--ov-shadow-sm);
  }

  .stage {
    color: var(--ov-text);
    background:
      radial-gradient(circle at center, rgba(91, 91, 214, 0.12), transparent 42%),
      linear-gradient(180deg, #fbfaff, #f4f3fc);
    border: 1px solid #e1e0f3;
    border-radius: var(--ov-radius-lg);
    box-shadow: var(--ov-shadow-sm);
  }

  .stage-timer {
    color: #171a28;
    font-size: 54px;
    font-weight: 650;
  }

  .stage-meta {
    color: var(--ov-text-secondary);
  }

  .stage video,
  video {
    border-radius: var(--ov-radius-md);
    background: #1c2030;
    box-shadow: var(--ov-shadow-sm);
  }

  button.ctl,
  .dialpad button {
    color: var(--ov-text-secondary);
    background: #fff;
    border-color: var(--ov-border-strong);
    box-shadow: var(--ov-shadow-sm);
  }

  button.ctl:hover:not(:disabled),
  .dialpad button:hover:not(:disabled) {
    color: var(--ov-primary);
    background: var(--ov-primary-soft);
    border-color: #c9c7f4;
  }

  button.ctl.on {
    color: #fff;
    background: var(--ov-primary);
    border-color: var(--ov-primary);
  }

  button.hangup {
    background: var(--ov-danger);
    border-color: var(--ov-danger);
    box-shadow: 0 5px 16px rgba(223, 82, 98, 0.2);
  }

  .stage-extra button,
  .stage-extra select {
    color: var(--ov-text-secondary);
    background: #fff;
    border-color: var(--ov-border-strong);
  }

  .guest-talk {
    max-width: 1080px;
    margin-inline: auto;
  }

  .h5-invite {
    max-width: 520px;
    margin: 5vh auto 0;
    padding: 36px;
    text-align: center;
  }

  .h5-invite-icon {
    width: 64px;
    height: 64px;
    margin: 0 auto 18px;
    display: grid;
    place-items: center;
    color: #fff;
    background: linear-gradient(135deg, var(--ov-primary), var(--ov-accent));
    border-radius: 20px;
    font-size: 30px;
  }

  .h5-invite h2 {
    margin: 0 0 10px;
  }

  .h5-checklist {
    margin: 24px 0;
    padding: 16px 20px 16px 38px;
    color: var(--ov-text-secondary);
    background: var(--ov-surface-soft);
    border-radius: var(--ov-radius-md);
    text-align: left;
  }

  .h5-start {
    width: 100%;
  }

  .queue-wait {
    max-width: 620px;
    margin: 0 auto;
    text-align: center;
  }

  .queue-position {
    width: 150px;
    height: 150px;
    margin: 18px auto 24px;
    padding: 9px;
    border-radius: 50%;
    background: conic-gradient(var(--ov-primary) 0 62%, var(--ov-accent) 62% 78%, #edf0f7 78%);
    box-shadow: 0 8px 24px rgba(91, 91, 214, 0.14);
  }

  .queue-position-inner {
    height: 100%;
    display: grid;
    place-content: center;
    border-radius: 50%;
    background: #fff;
  }

  .queue-position strong {
    display: block;
    color: var(--ov-text);
    font-size: 38px;
    line-height: 1;
  }

  .queue-position span {
    margin-top: 8px;
    color: var(--ov-text-secondary);
    font-size: 12px;
  }

  .acw-notes {
    flex: 1;
    min-width: 220px;
    min-height: 48px;
  }

  .wrap-notes {
    max-width: 480px;
    margin-top: var(--ov-space-3);
  }

  .side-panel {
    margin-bottom: var(--ov-space-4);
  }

  .queue-count {
    margin: 0;
    color: var(--ov-primary);
    font-size: 26px;
    font-weight: 650;
    font-variant-numeric: tabular-nums;
  }

  .queue-count-unit {
    color: var(--ov-text-muted);
    font-size: 13px;
    font-weight: 400;
  }

  .ivr-pad-label {
    margin: var(--ov-space-4) 0 var(--ov-space-2);
  }

  @media (max-width: 960px) {
    .workbench {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  @media (max-width: 640px) {
    .h5-invite {
      margin-top: 2vh;
      padding: 26px 20px;
    }

    .guest-talk .row {
      position: relative;
      display: block;
    }

    .guest-talk video#remote {
      min-height: 52vh;
      object-fit: cover;
    }

    .guest-talk video#local {
      position: absolute;
      right: 12px;
      bottom: 12px;
      width: 30%;
      max-width: 120px;
      border: 2px solid #fff;
    }

    .stage {
      padding: 22px 14px 16px;
    }

    .stage-videos {
      flex-direction: column;
    }

    .stage-videos video#local {
      max-width: 120px;
      align-self: flex-end;
    }

    .queue-position {
      width: 132px;
      height: 132px;
    }

    button.ctl {
      width: 52px;
      height: 52px;
      min-height: 52px;
    }
  }
`;
