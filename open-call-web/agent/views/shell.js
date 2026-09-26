// 本文件负责坐席界面的导航与页面骨架。
import { html } from "lit";
import { renderAppShell, renderFeedback, renderLoginLayout } from "../../shared/components/ui.js";
import { agentStateLabel, agentStateTone } from "../../shared/display.js";
import { isWebRTCSupported } from "../../shared/webrtc.js";
import { renderCredentialFields } from "../../shared/components/credentials.js";
import { renderStatusTag } from "../../shared/components/status-tag.js";
import { project } from "../../shared/view-model.js";
import { renderDeskView } from "./desk.js";
import { renderDeviceView } from "./device.js";
import { renderInboundView } from "./inbound.js";
import { renderOutboundView } from "./outbound.js";
import { renderQueueView } from "./queue.js";
export const NAV = [
  { id: "desk", label: "工作台" },
  { id: "inbound", label: "呼入" },
  { id: "outbound", label: "外呼" },
  { id: "queue", label: "队列签入" },
  { id: "device", label: "设备" },
];

export function renderApp(host, actions) {
    if (!host.me) return renderLoginView(host, actions);
    const state = host.me.session?.state || "offline";
    const can = (code) => host.permissions?.includes(code);
    const page = (render, stateKeys, actionKeys, ...rest) =>
      render(project(host, stateKeys), project(actions, actionKeys), ...rest);
    return renderAppShell({
      subtitle: "坐席工作台",
      navItems: NAV.filter((item) => ["inbound", "outbound"].includes(item.id) ? can("calls.operate") : item.id === "queue" ? can("agents.self") : true),
      activeNav: host.nav,
      onNavigate: actions.navigate,
      breadcrumb: actions.crumb(),
      badges: { inbound: host.incoming ? 1 : 0 },
      topbar: html`
          <span class="topbar-meta">${host.clock}</span>
          <span class="topbar-meta">${host.me.display_name || host.me.extension} · ${host.me.extension || ""}</span>
          ${renderStatusTag(agentStateLabel(state), agentStateTone(state))}
          <button @click=${() => actions.logout()}>退出</button>
      `,
      content: html`
        ${renderFeedback({ error: host.error, notice: host.notice })}
        ${host.nav === "desk" ? page(renderDeskView,
          ["audioMuted", "audioPlaybackBlocked", "call", "cameraUnavailable", "consulting", "dest", "elapsed", "hasLocal", "held", "incoming", "pendingWrapId", "permissions", "recentCalls", "sharing", "showPad", "videoAsk", "videoMuted", "wrapNotes", "xferMode"],
          ["answer", "askVideo", "completeXfer", "conf", "decline", "dial", "doCheckIn", "doCheckOut", "downgrade", "dtmf", "hangup", "hold", "playRemoteAudio", "respondVideo", "setDest", "setIdle", "setShowPad", "setWrapNotes", "setXferMode", "share", "stageCaller", "stageQueue", "submitWrap", "toggleBusy", "toggleMute", "waitingCount", "xfer"], state) : ""}
        ${host.nav === "inbound" ? page(renderInboundView, ["incoming", "permissions"], ["answer", "decline"]) : ""}
        ${host.nav === "outbound" ? page(renderOutboundView,
          ["dest", "guestExpiresAt", "guestLink", "guestMedia", "me", "permissions"],
          ["copyGuestLink", "dial", "listen", "makeLink", "setDest", "setGuestMedia"]) : ""}
        ${host.nav === "queue" ? page(renderQueueView,
          ["busyReason", "queues", "selectedQueues"],
          ["doCheckIn", "doCheckOut", "setBusyReason", "setSelectedQueues", "toggleBusy"], state) : ""}
        ${host.nav === "device" ? page(renderDeviceView,
          ["audioDeviceId", "call", "devices", "hasLocal", "speakerDeviceId", "videoDeviceId"],
          ["onCamChange", "onMicChange", "onSpeakerChange", "preview"]) : ""}
      `,
    });
  }

export function renderLoginView(host, actions) {
    return renderLoginLayout({
      subtitle: "坐席工作台",
      title: "坐席登录",
      hint: `WebRTC：${isWebRTCSupported() ? "支持" : "不支持"}`,
      onSubmit: (event) => actions.login(event),
      error: host.error,
      fields: html`
        ${!host.authOptions || host.authOptions.unavailable || host.authOptions.oidc_enabled ? "" : html`
          ${renderCredentialFields({ prefix: "agent", username: host.username, password: host.password,
            onUsernameChange: actions.setUsername, onPasswordChange: actions.setPassword })}
        `}
      `,
      showSubmit: !!host.authOptions && !host.authOptions.unavailable && !host.authOptions.oidc_enabled,
      extraActions: host.authOptions?.oidc_enabled ? html`<button type="button" @click=${() => actions.startSSO()}>统一身份平台登录</button>` : "",
    });
  }
