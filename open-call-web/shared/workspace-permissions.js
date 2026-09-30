// 管理工作区 / 页面权限：单一事实来源。

/** 管理端各导航页对应的读权限（identity 见 IDENTITY_PAGE_PERMISSIONS）。 */
export const ADMIN_PAGE_PERMISSIONS = {
  overview: "status.read",
  runtime: "calls.read",
  queues: "queues.read",
  agents: "agents.read",
  dids: "dids.read",
  cdr: "cdr.read",
  recordings: "recordings.read",
  ivr: "ivr.read",
  webhooks: "webhooks.read",
  audit: "audit.read",
};

/** 用户与权限页：任一即可进入。 */
export const IDENTITY_PAGE_PERMISSIONS = [
  "users.read",
  "users.create",
  "roles.read",
  "identity.read",
];

/** 能进入管理工作区的权限并集（含话务读权限）。 */
export const ADMIN_WORKSPACE_PERMISSIONS = [
  ...new Set([
    ...Object.values(ADMIN_PAGE_PERMISSIONS),
    ...IDENTITY_PAGE_PERMISSIONS,
  ]),
];

export function hasPermission(permissions, code) {
  return (permissions || []).includes(code);
}

export function canOpenAdminPage(permissions, pageId) {
  if (pageId === "identity") {
    return IDENTITY_PAGE_PERMISSIONS.some((code) => hasPermission(permissions, code));
  }
  const code = ADMIN_PAGE_PERMISSIONS[pageId];
  return !!code && hasPermission(permissions, code);
}

/** 返回第一个可打开的管理页 id，无则空串。 */
export function firstAdminPage(permissions) {
  for (const id of Object.keys(ADMIN_PAGE_PERMISSIONS)) {
    if (canOpenAdminPage(permissions, id)) return id;
  }
  if (canOpenAdminPage(permissions, "identity")) return "identity";
  return "";
}

export function availableViews(identity) {
  const permissions = identity?.permissions || [];
  return {
    admin: ADMIN_WORKSPACE_PERMISSIONS.some((code) => permissions.includes(code)),
    agent: !!identity?.agent_id && permissions.includes("agents.self"),
  };
}
