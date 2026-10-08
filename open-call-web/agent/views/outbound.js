// 本文件负责坐席外呼处理视图。
import { html } from "lit";
import { renderPanel } from "../../shared/components/panel.js";
import { renderDialControls } from "../components/dial-controls.js";
import { isMediaDevicesAvailable, mediaDevicesUnavailableMessage } from "../../shared/webrtc.js";

export function renderOutboundView(model, actions) {
    return html`
        <div class="form-inline">
          ${model.permissions?.includes("calls.operate") ? renderDialControls(model.dest, actions.setDest, actions.dial) : ""}
          ${model.permissions?.includes("guest.issue") ? html`
          <button class="secondary" @click=${() => actions.makeLink()}>语音入会链接</button>` : ""}
          ${model.permissions?.includes("calls.listen")
            ? html`<button class="secondary" @click=${() => actions.listen()}>监听（填 call_id）</button>`
            : ""}
        </div>
        ${model.permissions?.includes("calls.operate") ? renderPanel("语音通知", html`
          <p class="hint">提交后由平台出局播放素材并自动挂断，关闭页面不影响进行中的通知。</p>
          <label>语音素材
            <select .value=${model.promptAssetId || ""} @change=${(e) => actions.setPromptAssetId(e.target.value)} ?disabled=${model.voiceNotifyBusy}>
              <option value="">请选择…</option>
              ${(model.ivrAssets || []).map((a) => html`<option value=${a.id}>${a.name}</option>`)}
            </select>
          </label>
          <button class="primary" ?disabled=${model.voiceNotifyBusy || !model.dest?.trim() || !model.promptAssetId} @click=${() => actions.voiceNotify()}>发起语音通知</button>
          ${model.notificationTask ? html`
            <p>最近通知：${model.notificationTask.destination} · ${{ queued: "等待执行", dialing: "正在拨号", playing: "正在播放", finishing: "正在结束", stopping: "正在取消或清理", completed: "播放完成", canceled: "已取消", failed: "失败" }[model.notificationTask.state] || model.notificationTask.state}</p>
            ${model.notificationTask.error ? html`<p class="hint">${model.notificationTask.error}</p>` : ""}
            <button class="secondary" @click=${() => actions.refreshNotification()}>刷新通知状态</button>
            ${!["completed", "canceled", "failed"].includes(model.notificationTask.state) ? html`<button class="secondary" @click=${() => actions.cancelNotification()}>取消通知</button>` : ""}
          ` : ""}
        `) : ""}
        ${model.guestLink ? html`
          <div class="invite-link">
            <label for="guest-link">H5 访客链接${model.guestExpiresAt ? `（有效至 ${model.guestExpiresAt}）` : ""}</label>
            <input id="guest-link" readonly .value=${model.guestLink} />
            <button class="secondary" @click=${() => actions.copyGuestLink()}>复制链接</button>
          </div>` : ""}
        <p class="hint">分机互拨填写对方分机；SIP 软电话填写 bob；PSTN 填 8 位以上号码。</p>
        ${!isMediaDevicesAvailable() ? html`<p class="hint" style="color:var(--ov-danger,#c41)">${mediaDevicesUnavailableMessage()}</p>` : ""}
    `;
  }
