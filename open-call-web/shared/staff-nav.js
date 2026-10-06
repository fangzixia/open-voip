// 坐席 + 管理：统一侧栏菜单（权限过滤）。
import { NAV as AGENT_NAV } from "../agent/views/shell.js";
import { NAV as ADMIN_NAV } from "../admin/views/shell.js";
import { canOpenAdminPage } from "./workspace-permissions.js";

export const AGENT_NAV_IDS = new Set(AGENT_NAV.map((item) => item.id));

export function isAgentNav(id) {
  return AGENT_NAV_IDS.has(id);
}

export function isAdminNav(id) {
  return !!id && !isAgentNav(id);
}

/** @param {{ permissions?: string[], agent_id?: string }} identity */
export function buildStaffNav(identity) {
  const perms = identity?.permissions || [];
  const can = (code) => perms.includes(code);
  const items = [];

  if (identity?.agent_id && can("agents.self")) {
    for (const item of AGENT_NAV) {
      if (["inbound", "outbound"].includes(item.id) && !can("calls.operate")) continue;
      if (item.id === "queue" && !can("agents.self")) continue;
      items.push({ ...item, group: item.group || "坐席" });
    }
  }

  for (const item of ADMIN_NAV) {
    if (canOpenAdminPage(perms, item.id)) items.push({ ...item });
  }
  return items;
}

export function staffNavLabel(navItems, activeId) {
  return navItems.find((n) => n.id === activeId)?.label || "";
}

export function defaultStaffNavId(navItems) {
  return navItems[0]?.id || "desk";
}
