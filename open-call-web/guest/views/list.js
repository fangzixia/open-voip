// 本文件负责访客服务列表视图。
import { html } from "lit";
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderPanel } from "../../shared/components/panel.js";
export function renderListView(state, actions) {
    return renderPanel("可进入队列", renderDataTable([
      { label: "队列名称", key: "name" },
      { label: "类型", render: (q) => q.video_enabled ? "语音 / 视频" : "语音" },
      { label: "操作", render: (q) => html`<button class="ghost" @click=${() => actions.start(q, false)}>进入</button>` },
    ], state.queues, { label: "可进入队列" }));
  }
