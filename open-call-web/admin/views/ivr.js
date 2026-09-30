// 本文件负责 IVR 管理页；整页设计器独立于 CRUD 模板。
import { html } from "lit";
import { ivrService } from "../controllers/ivr.js";
import { renderPage } from "../../shared/components/page-layout.js";

export function renderIvr(state, actions) {
  return renderPage({
    description: "IVR 流程设计器：编辑草稿、发布版本，并绑定到呼入语音队列。",
    children: html`<div class="page-ivr"><ivr-flow-editor .flows=${state.ivrs} .queues=${state.queues} .service=${ivrService} @ivr-changed=${() => actions.load()}></ivr-flow-editor></div>`,
  });
}
