// 本文件负责坐席呼入处理视图。
import { html } from "lit";
import { renderIncomingBar } from "./desk.js";
import { renderPanel } from "../../shared/components/panel.js";
export function renderInboundView(model, actions) {
    return renderPanel("呼入", html`
        ${model.incoming
          ? renderIncomingBar(model, actions)
          : html`<p class="muted">当前没有振铃来电。签入队列后，来电会显示在此处，也可在工作台接听。</p>`}
    `);
  }
