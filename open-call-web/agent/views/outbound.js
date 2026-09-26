// 本文件负责坐席外呼处理视图。
import { html } from "lit";
import { renderPanel } from "../../shared/components/panel.js";
import { renderDialControls } from "../components/dial-controls.js";
export function renderOutboundView(model, actions) {
    return renderPanel("外呼", html`
        <div class="form-inline">
          ${model.permissions?.includes("calls.operate") ? renderDialControls(model.dest, actions.setDest, actions.dial) : ""}
          ${model.permissions?.includes("guest.issue") ? html`<select aria-label="访客邀请媒体" .value=${model.guestMedia} @change=${(e) => actions.setGuestMedia(e.target.value)}>
            <option value="video">视频邀请</option>
            <option value="audio">语音邀请</option>
          </select>
          <button class="secondary" @click=${() => actions.makeLink()}>入会链接</button>` : ""}
          ${model.permissions?.includes("calls.listen")
            ? html`<button class="secondary" @click=${() => actions.listen()}>监听（填 call_id）</button>`
            : ""}
        </div>
        ${model.guestLink ? html`
          <div class="invite-link">
            <label for="guest-link">H5 访客链接${model.guestExpiresAt ? `（有效至 ${model.guestExpiresAt}）` : ""}</label>
            <input id="guest-link" readonly .value=${model.guestLink} />
            <button class="secondary" @click=${() => actions.copyGuestLink()}>复制链接</button>
          </div>` : ""}
        <p class="hint">分机互拨填写对方分机；SIP 软电话填写 bob；PSTN 填 8 位以上号码。</p>
    `);
  }
