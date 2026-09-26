// 本文件负责IVR 管理页，展示流程和发布操作。
import { html } from "lit";
import { ivrService } from "../controllers/ivr.js";
export function renderIvr(state, actions) {
    return html`<ivr-flow-editor .flows=${state.ivrs} .queues=${state.queues} .service=${ivrService} @ivr-changed=${() => actions.load()}></ivr-flow-editor>`;
  }
