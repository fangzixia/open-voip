import { html } from "lit";
import { renderField } from "./panel.js";

/** 登录页面共用的用户名与密码输入控件。 */
export function renderCredentialFields({ prefix, username, password, onUsernameChange, onPasswordChange }) {
  const usernameId = `${prefix}-username`;
  const passwordId = `${prefix}-password`;
  return html`
    ${renderField("用户名", html`<input id=${usernameId} autocomplete="username" .value=${username} @input=${(event) => onUsernameChange(event.target.value)} />`, { htmlFor: usernameId })}
    ${renderField("密码", html`<input id=${passwordId} type="password" autocomplete="current-password" .value=${password} @input=${(event) => onPasswordChange(event.target.value)} />`, { htmlFor: passwordId })}
  `;
}
