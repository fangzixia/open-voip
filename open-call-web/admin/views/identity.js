// 本文件负责身份管理页，展示用户、角色和外部身份映射。
import { html } from "lit";
import { renderPanel, renderField } from "../../shared/components/panel.js";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderCheckbox } from "../../shared/components/checkbox.js";

const can = (state, code) => state.me?.permissions?.includes(code);

export function renderIdentity(state, actions) {
  const selected = state.users.find((user) => user.id === state.selectedUser);
  return html`
    ${can(state, "users.create") ? renderPanel("新建用户 / 坐席", html`<form @submit=${(event) => { event.preventDefault(); actions.createIdentityUser(); }}>
      ${renderField("用户名", html`<input required .value=${state.newIdentityUser.username} @input=${(event) => actions.updateNewIdentityUser({ username: event.target.value })} />`)}
      ${renderField("登录名", html`<input required .value=${state.newIdentityUser.login_name} @input=${(event) => actions.updateNewIdentityUser({ login_name: event.target.value })} />`)}
      ${renderField("工号", html`<input required .value=${state.newIdentityUser.employee_no} @input=${(event) => actions.updateNewIdentityUser({ employee_no: event.target.value })} />`)}
      ${renderField("初始密码", html`<input type="password" required .value=${state.newIdentityUser.password} @input=${(event) => actions.updateNewIdentityUser({ password: event.target.value })} />`)}
      ${can(state, "roles.read") ? html`<div class="form-inline">${state.roles.map((role) => renderCheckbox(role.name,
        state.newIdentityUser.roles.includes(role.id), (checked) => actions.toggleNewUserRole(role.id, checked)))}</div>`
        : renderField("角色", html`<select .value=${state.newIdentityUser.role} @change=${(event) => actions.updateNewIdentityUser({ role: event.target.value })}>
            <option value="agent">坐席</option><option value="supervisor">班长</option><option value="admin">管理员</option>
          </select>`)}
      <h4>坐席资料${!can(state, "roles.read") && state.newIdentityUser.role === "agent" ? "" : "（可选）"}</h4>
      ${renderField("分机号", html`<input .value=${state.newIdentityUser.extension}
        ?required=${state.newIdentityUser.terminal_type === "sip" || (!can(state, "roles.read") && state.newIdentityUser.role === "agent")}
        @input=${(event) => actions.updateNewIdentityUser({ extension: event.target.value })} />`)}
      ${renderField("终端类型", html`<select .value=${state.newIdentityUser.terminal_type} @change=${(event) => actions.updateNewIdentityUser({ terminal_type: event.target.value })}>
        <option value="webrtc">WebRTC</option><option value="sip">SIP</option>
      </select>`)}
      ${state.newIdentityUser.terminal_type === "webrtc" ? renderCheckbox("支持视频",
        state.newIdentityUser.video_capable, (checked) => actions.updateNewIdentityUser({ video_capable: checked })) : ""}
      <button type="submit">创建用户</button>
    </form>`) : ""}
    ${can(state, "users.read") ? renderPanel("用户", renderDataTable([
      { label: "用户名", key: "username" },
      { label: "登录名", key: "login_name" },
      { label: "工号", key: "employee_no" },
      { label: "角色", render: (user) => (user.roles || []).join("、") },
      { label: "状态", render: (user) => user.disabled ? "已禁用" : "启用" },
      { label: "操作", render: (user) => html`<button @click=${() => actions.selectUser(user.id)}>管理</button>` },
    ], state.users, { label: "用户列表", showCount: true })) : ""}

    ${selected ? renderPanel(`管理用户：${selected.username}`, html`
      <div class="form-inline">
        ${can(state, "users.update") ? html`
          ${renderField("用户名", html`<input .value=${selected.username || ""} @change=${(event) => actions.saveUsername(selected.id, event.target.value)} />`)}
          ${renderField("登录名", html`<input .value=${selected.login_name || ""} @change=${(event) => actions.saveLoginName(selected.id, event.target.value)} />`)}
          ${renderField("工号", html`<input .value=${selected.employee_no || ""} @change=${(event) => actions.saveEmployeeNo(selected.id, event.target.value)} />`)}
        ` : ""}
        ${can(state, "users.update") ? html`<button @click=${() => actions.toggleDisabled(selected)}>${selected.disabled ? "启用账号" : "禁用账号"}</button>` : ""}
        ${can(state, "users.sessions") ? html`<button @click=${() => actions.revokeSessions(selected.id)}>撤销全部会话</button>` : ""}
      </div>

      ${can(state, "users.update") ? html`
        <h4>角色</h4>
        ${state.roles.map((role) => renderCheckbox(role.name,
          (selected.roles || []).includes(role.id), (checked) => actions.toggleUserRole(selected.id, role.id, checked)))}
        <button @click=${() => actions.saveUserRoles(selected.id)}>保存角色</button>
        <h4>坐席资料</h4>
        <p>${selected.agent_id ? `坐席编号：${selected.agent_id}` : "设置分机和终端后，该用户才能作为坐席签入。"}</p>
        <form @submit=${(event) => { event.preventDefault(); actions.saveAgentProfile(selected.id); }}>
          ${renderField("分机号", html`<input .value=${state.agentProfileDraft.extension} @input=${(event) => actions.updateAgentProfile({ extension: event.target.value })} />`)}
          ${renderField("终端类型", html`<select .value=${state.agentProfileDraft.terminal_type} @change=${(event) => actions.updateAgentProfile({ terminal_type: event.target.value })}>
            <option value="webrtc">WebRTC</option><option value="sip">SIP</option>
          </select>`)}
          ${state.agentProfileDraft.terminal_type === "webrtc" ? renderCheckbox("支持视频",
            state.agentProfileDraft.video_capable, (checked) => actions.updateAgentProfile({ video_capable: checked })) : ""}
          <button type="submit">保存坐席资料</button>
        </form>
      ` : ""}

      ${can(state, "identity.read") ? html`
        <h4>外部身份绑定</h4>
        ${state.identities.map((identity) => html`<p>${identity.issuer} · ${identity.subject} ${can(state, "identity.write") ? html`<button @click=${() => actions.unbindIdentity(selected.id, identity.issuer, identity.subject)}>解除绑定</button>` : ""}</p>`)}
        ${can(state, "identity.write") ? html`<form @submit=${(event) => { event.preventDefault(); actions.bindIdentity(selected.id); }}>
          ${renderField("Issuer", html`<input .value=${state.identityInput.issuer} @input=${(event) => actions.updateIdentityInput({ issuer: event.target.value })} />`)}
          ${renderField("Subject", html`<input .value=${state.identityInput.subject} @input=${(event) => actions.updateIdentityInput({ subject: event.target.value })} />`)}
          <button type="submit">绑定身份</button>
        </form>` : ""}
      ` : ""}
    `) : ""}

    ${can(state, "roles.read") ? renderPanel("角色与权限", html`
      ${renderDataTable([
        { label: "编号", key: "id" },
        { label: "名称", key: "name" },
        { label: "权限数", render: (role) => role.permissions.length },
        { label: "操作", render: (role) => role.built_in ? "内置" : html`<button @click=${() => actions.editRole(role)}>编辑</button>` },
      ], state.roles, { label: "角色列表" })}
      ${can(state, "roles.write") ? html`<form @submit=${(event) => { event.preventDefault(); actions.saveRole(); }}>
        ${renderField("角色编号", html`<input .value=${state.newRole.id} @input=${(event) => actions.updateRole({ id: event.target.value })} />`)}
        ${renderField("名称", html`<input .value=${state.newRole.name} @input=${(event) => actions.updateRole({ name: event.target.value })} />`)}
        <div class="form-inline">${state.permissionsCatalog.map((permission) => renderCheckbox(permission.description,
          state.newRole.permissions.includes(permission.code), (checked) => actions.togglePermission(permission.code, checked)))}</div>
        <button type="submit">保存角色</button>
        ${state.newRole.id && !state.roles.find((role) => role.id === state.newRole.id)?.built_in
          ? html`<button type="button" @click=${() => actions.deleteRole()}>删除角色</button>` : ""}
      </form>` : ""}
    `) : ""}

    ${can(state, "identity.read") ? renderPanel("外部组映射", html`
      ${renderDataTable([
        { label: "外部组", key: "group_name" },
        { label: "本项目角色", key: "role_id" },
      ], state.groupMappings, { label: "组映射" })}
      ${can(state, "identity.write") ? html`<form @submit=${(event) => { event.preventDefault(); actions.saveMapping(); }}>
        ${renderField("外部组名", html`<input .value=${state.newMapping.group} @input=${(event) => actions.updateMapping({ group: event.target.value })} />`)}
        ${state.roles.map((role) => renderCheckbox(role.name,
          state.newMapping.roles.includes(role.id), (checked) => actions.toggleMappingRole(role.id, checked)))}
        <button type="submit">保存映射</button>
      </form>` : ""}
    `) : ""}
  `;
}
