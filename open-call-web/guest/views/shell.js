// 本文件负责访客界面的页面骨架。
import { html } from "lit";
import { renderAppShell, renderFeedback } from "../../shared/components/ui.js";
import { renderPanel } from "../../shared/components/panel.js";
import { project } from "../../shared/view-model.js";
import { renderListView } from "./list.js";
import { renderQueueIdleView } from "./queue.js";
import { renderWaitView } from "./queue.js";
import { renderInviteView } from "./service.js";
import { renderPickView } from "./service.js";
import { renderEndedView } from "./talk.js";
import { renderTalkView } from "./talk.js";
export const NAV = [
  { id: "service", label: "选择服务" },
  { id: "queue", label: "排队状态" },
  { id: "talk", label: "当前通话" },
  { id: "list", label: "可进入队列" },
];

export function renderApp(host, actions) {
    const waiting = host.step === "perm" || host.step === "wait";
    const page = (render, stateKeys, actionKeys) => render(project(host, stateKeys), project(actions, actionKeys));
    return renderAppShell({
      subtitle: "在线客服",
      navItems: NAV,
      activeNav: host.nav,
      onNavigate: (id) => actions.go(id),
      breadcrumb: actions.crumb(),
      badges: { queue: waiting ? 1 : 0 },
      minimal: true,
      content: html`
        ${renderFeedback({ error: host.error })}
        ${host.nav === "service" ? (host.step === "invite"
          ? page(renderInviteView, ["inviteMedia"], ["startToken"])
          : host.step === "ended"
            ? page(renderEndedView, [], ["restart"])
            : page(renderPickView, ["queues", "selected", "userId"], ["pick", "start", "setUserId"])) : ""}
        ${host.nav === "queue" ? (waiting
          ? page(renderWaitView, ["audioPlaybackBlocked", "permissionHint", "position", "waitSec", "notice"], ["playRemoteAudio", "dtmf", "hangup"])
          : renderQueueIdleView()) : ""}
        ${host.nav === "talk" ? (host.step === "talk"
          ? page(renderTalkView, ["elapsed", "notice", "videoAsk", "audioPlaybackBlocked", "wantVideo", "audioMuted", "videoMuted"], ["respondVideo", "playRemoteAudio", "toggle", "switchCamera", "hangup", "dtmf"])
          : host.step === "ended"
            ? page(renderEndedView, [], ["restart"])
            : renderPanel("当前通话", html`<div class="empty-state">当前没有通话，请先在「选择服务」发起。</div>`)) : ""}
        ${host.nav === "list" ? page(renderListView, ["queues"], ["start"]) : ""}
      `,
    });
  }
