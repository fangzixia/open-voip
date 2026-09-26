import {
  bindIdentity, createUser, deleteRole, listIdentities, patchUser, revokeUserSessions,
  saveGroupMapping, saveRole, setUserRoles, unbindIdentity,
} from "../../shared/api.js";

export function newIdentityUserDraft() {
  return { username: "", login_name: "", employee_no: "", password: "", roles: [], role: "agent", extension: "", terminal_type: "webrtc", video_capable: true };
}

export function createUserPayload(draft, canReadRoles) {
  const extension = draft.extension.trim();
  return {
    username: draft.username, login_name: draft.login_name, employee_no: draft.employee_no, password: draft.password,
    ...(canReadRoles ? { roles: draft.roles } : { role: draft.role }),
    extension, terminal_type: draft.terminal_type,
    sip_username: draft.terminal_type === "sip" && extension ? extension : "",
    video_capable: draft.terminal_type === "webrtc" && draft.video_capable,
  };
}

/** 管理身份信息草稿与请求，并向视图提供必要的操作回调。 */
export function identityActions(host, reload) {
  const refreshIdentities = async () => {
    if (host.selectedUser && host.me?.permissions?.includes("identity.read")) {
      host.identities = (await listIdentities(host.selectedUser)).items || [];
    }
  };
  const run = async (request) => {
    try {
      await request();
      await reload();
      await refreshIdentities();
      host.error = "";
    } catch (error) {
      host.error = error instanceof Error ? error.message : String(error);
    }
  };
  return {
    updateNewIdentityUser: (patch) => { host.newIdentityUser = { ...host.newIdentityUser, ...patch }; },
    toggleNewUserRole: (role, checked) => {
      const roles = new Set(host.newIdentityUser.roles);
      if (checked) roles.add(role);
      else roles.delete(role);
      host.newIdentityUser = { ...host.newIdentityUser, roles: [...roles] };
    },
    createIdentityUser: () => {
      const draft = host.newIdentityUser;
      const canReadRoles = host.me?.permissions?.includes("roles.read");
      if (canReadRoles && !draft.roles.length) {
        host.error = "请至少选择一个角色";
        return;
      }
      return run(async () => {
        const payload = createUserPayload(draft, canReadRoles);
        const user = await createUser(payload);
        host.selectedUser = user.id;
        host.agentProfileDraft = { extension: payload.extension, terminal_type: payload.terminal_type, video_capable: payload.video_capable };
        host.newIdentityUser = newIdentityUserDraft();
      });
    },
    selectUser: async (id) => {
      host.selectedUser = id;
      host.identities = [];
      const user = host.users.find((item) => item.id === id);
      host.agentProfileDraft = {
        extension: user?.extension || "",
        terminal_type: user?.terminal_type || "webrtc",
        video_capable: !!user?.video_capable,
      };
      try { await refreshIdentities(); }
      catch (error) { host.error = error instanceof Error ? error.message : String(error); }
    },
    saveUsername: (id, username) => run(() => patchUser(id, { username })),
    saveLoginName: (id, login_name) => run(() => patchUser(id, { login_name })),
    saveEmployeeNo: (id, employee_no) => run(() => patchUser(id, { employee_no })),
    updateAgentProfile: (patch) => { host.agentProfileDraft = { ...host.agentProfileDraft, ...patch }; },
    saveAgentProfile: (id) => run(() => patchUser(id, {
      ...host.agentProfileDraft,
      sip_username: host.agentProfileDraft.terminal_type === "sip" ? host.agentProfileDraft.extension : "",
      video_capable: host.agentProfileDraft.terminal_type === "sip" ? false : host.agentProfileDraft.video_capable,
    })),
    toggleDisabled: (user) => run(() => patchUser(user.id, { disabled: !user.disabled })),
    revokeSessions: (id) => run(() => revokeUserSessions(id)),
    toggleUserRole: (id, role, checked) => {
      host.users = host.users.map((user) => {
        if (user.id !== id) return user;
        const roles = new Set(user.roles || []);
        if (checked) roles.add(role);
        else roles.delete(role);
        return { ...user, roles: [...roles] };
      });
    },
    saveUserRoles: (id) => run(() => setUserRoles(id, host.users.find((user) => user.id === id)?.roles || [])),
    updateIdentityInput: (patch) => { host.identityInput = { ...host.identityInput, ...patch }; },
    bindIdentity: (id) => run(() => bindIdentity(id, host.identityInput.issuer, host.identityInput.subject)),
    unbindIdentity: (id, issuer, subject) => run(() => unbindIdentity(id, issuer, subject)),
    editRole: (role) => { host.newRole = { id: role.id, name: role.name, permissions: [...role.permissions] }; },
    updateRole: (patch) => { host.newRole = { ...host.newRole, ...patch }; },
    togglePermission: (code, checked) => {
      const permissions = new Set(host.newRole.permissions);
      if (checked) permissions.add(code);
      else permissions.delete(code);
      host.newRole = { ...host.newRole, permissions: [...permissions] };
    },
    saveRole: () => run(() => saveRole(host.newRole.id, host.newRole.name, host.newRole.permissions)),
    deleteRole: () => run(async () => {
      await deleteRole(host.newRole.id);
      host.newRole = { id: "", name: "", permissions: [] };
    }),
    updateMapping: (patch) => { host.newMapping = { ...host.newMapping, ...patch }; },
    toggleMappingRole: (role, checked) => {
      const roles = new Set(host.newMapping.roles);
      if (checked) roles.add(role);
      else roles.delete(role);
      host.newMapping = { ...host.newMapping, roles: [...roles] };
    },
    saveMapping: () => run(() => saveGroupMapping(host.newMapping.group, host.newMapping.roles)),
  };
}
