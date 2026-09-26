const ADMIN_PERMISSIONS = [
  "status.read", "users.read", "users.create", "roles.read", "identity.read",
  "dids.read", "cdr.read", "recordings.read", "ivr.read", "webhooks.read", "audit.read",
];

export function availableViews(identity) {
  const permissions = identity?.permissions || [];
  return {
    admin: ADMIN_PERMISSIONS.some((code) => permissions.includes(code)),
    agent: !!identity?.agent_id && permissions.includes("agents.self"),
  };
}
