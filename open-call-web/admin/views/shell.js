// 本文件负责管理界面的导航与页面骨架。
import { html } from "lit";
import { renderAppShell, renderFeedback, renderLoginLayout } from "../../shared/components/ui.js";
import { renderCredentialFields } from "../../shared/components/credentials.js";
import { project } from "../../shared/view-model.js";
import { adminViewActions } from "../controllers/view-actions.js";
import { renderAgents } from "./agents.js";
import { renderAudit } from "./audit.js";
import { renderCdrPage } from "./cdr.js";
import { renderDids } from "./dids.js";
import { renderIvr } from "./ivr.js";
import { renderOverview } from "./overview.js";
import { renderQueues } from "./queues.js";
import { renderRecs } from "./recordings.js";
import { renderHooks } from "./webhooks.js";
import { renderIdentity } from "./identity.js";

export const NAV = [
  { id: "overview", label: "总览", group: "功能导航" },
  { id: "queues", label: "队列", group: "话务管理" },
  { id: "agents", label: "坐席", group: "话务管理" },
  { id: "dids", label: "呼入号码", group: "话务管理" },
  { id: "cdr", label: "通话记录", group: "话务管理" },
  { id: "recordings", label: "录音", group: "话务管理" },
  { id: "ivr", label: "IVR", group: "流程管理" },
  { id: "webhooks", label: "Webhook", group: "流程管理" },
  { id: "audit", label: "审计", group: "系统设置" },
  { id: "identity", label: "用户与权限", group: "系统设置" },
];

const pagePermissions = {
  overview: "status.read", queues: "queues.read", agents: "agents.read", dids: "dids.read",
  cdr: "cdr.read", recordings: "recordings.read", ivr: "ivr.read",
  webhooks: "webhooks.read", audit: "audit.read",
};

export function renderApp(host, operations) {
  const actions = adminViewActions(host, operations);
  if (!host.authed) {
    return renderLoginLayout({
      subtitle: "管理控台",
      title: "管理员登录",
      hint: host.authOptions?.oidc_enabled ? "员工使用统一身份平台；密码入口仅供应急管理员" : "请使用部署时配置的管理员账号登录",
      onSubmit: actions.login,
      error: host.error,
      fields: !host.authOptions || host.authOptions.unavailable ? "" : renderCredentialFields({ prefix: "admin", username: host.username, password: host.password,
        onUsernameChange: actions.setUsername, onPasswordChange: actions.setPassword }),
      showSubmit: !!host.authOptions && !host.authOptions.unavailable,
      extraActions: host.authOptions?.oidc_enabled ? html`<button type="button" @click=${actions.startSSO}>统一身份平台登录</button>` : "",
    });
  }

  const page = (render, stateKeys, actionKeys) =>
    render(project(host, stateKeys), project(actions, actionKeys));
  return renderAppShell({
    subtitle: "管理控台",
    navItems: NAV.filter((item) => item.id === "identity"
      ? ["users.read", "users.create", "roles.read", "identity.read"].some((p) => host.me?.permissions?.includes(p))
      : host.me?.permissions?.includes(pagePermissions[item.id])),
    activeNav: host.nav,
    onNavigate: actions.navigate,
    breadcrumb: actions.crumb(),
    topbar: html`
      <span class="topbar-meta">${host.clock}</span>
      <span class="topbar-meta">管理员</span>
      <button @click=${actions.logout}>退出</button>
    `,
    content: html`
      ${renderFeedback({ error: host.error })}
      ${host.nav === "overview" ? page(renderOverview,
        ["agents", "cdr", "cdrCaller", "cdrResult", "hist", "live", "me", "queues", "status"],
        ["exportCdr", "filteredCdr", "idleAgents", "navigate", "resetCdr", "search", "setCdrCaller", "setCdrResult", "waiting"]) : ""}
      ${host.nav === "queues" ? page(renderQueues,
        ["agents", "me", "newQueue", "queues", "skillName", "skills"],
        ["addSkill", "bindAll", "bindSkill", "createQueue", "setBindAgentId", "setBindSkillId", "setSkillName", "toggleVip", "updateNewQueue"]) : ""}
      ${host.nav === "agents" ? page(renderAgents,
        ["agents", "hist", "me", "utils"],
        ["force", "navigate", "setForceAgentId"]) : ""}
      ${host.nav === "dids" ? page(renderDids,
        ["didForm", "dids", "me"], ["saveDid", "updateDidForm"]) : ""}
      ${host.nav === "cdr" ? page(renderCdrPage,
        ["cdrCaller", "cdrResult", "me", "qaCallId", "qaLabel", "wrapUps"],
        ["exportCdr", "filteredCdr", "qa", "resetCdr", "search", "setCdrCaller", "setCdrResult", "setQaCallId", "setQaLabel"]) : ""}
      ${host.nav === "recordings" ? page(renderRecs,
        ["me", "playType", "playUrl", "recs"], ["downloadRec", "playRec"]) : ""}
      ${host.nav === "ivr" ? page(renderIvr, ["ivrs", "queues"], ["load"]) : ""}
      ${host.nav === "webhooks" ? page(renderHooks,
        ["hookUrl", "hooks", "me"], ["addHook", "setHookUrl"]) : ""}
      ${host.nav === "audit" ? page(renderAudit, ["audit"], []) : ""}
      ${host.nav === "identity" ? page(renderIdentity,
        ["agentProfileDraft", "groupMappings", "identities", "identityInput", "me", "newIdentityUser", "newMapping", "newRole", "permissionsCatalog", "roles", "selectedUser", "users"],
        ["bindIdentity", "createIdentityUser", "deleteRole", "editRole", "revokeSessions", "saveAgentProfile", "saveDisplayName", "saveEmail", "saveMapping", "saveRole", "saveUserRoles", "selectUser", "toggleDisabled", "toggleMappingRole", "toggleNewUserRole", "togglePermission", "toggleUserRole", "unbindIdentity", "updateAgentProfile", "updateIdentityInput", "updateMapping", "updateNewIdentityUser", "updateRole"]) : ""}
    `,
  });
}
