// 本文件负责坐席设备设置页，管理麦克风和摄像头。
import { html } from "lit";
import { renderField, renderPanel } from "../../shared/components/panel.js";
export function renderDeviceView(model, actions) {
    return renderPanel("设备", html`
        <div class="form-inline">
          ${renderField("麦克风", html`<select @change=${(e) => actions.onMicChange(e.target.value)}>
              ${model.devices.audioInputs.map((d) => html`<option value=${d.deviceId} ?selected=${d.deviceId === model.audioDeviceId}>${d.label || d.deviceId}</option>`)}
            </select>`)}
          ${renderField("摄像头", html`<select @change=${(e) => actions.onCamChange(e.target.value)}>
              ${model.devices.videoInputs.map((d) => html`<option value=${d.deviceId} ?selected=${d.deviceId === model.videoDeviceId}>${d.label || d.deviceId}</option>`)}
            </select>`)}
          ${renderField("扬声器", html`<select @change=${(e) => actions.onSpeakerChange(e.target.value)}>
              ${model.devices.audioOutputs.map((d) => html`<option value=${d.deviceId} ?selected=${d.deviceId === model.speakerDeviceId}>${d.label || d.deviceId || "默认"}</option>`)}
            </select>`)}
          <button class="secondary" ?disabled=${!model.devices.videoInputs.length} @click=${() => actions.preview()}>预览摄像头</button>
        </div>
        ${model.hasLocal && !model.call ? html`<video id="local" autoplay muted playsinline></video>` : ""}
    `);
  }
