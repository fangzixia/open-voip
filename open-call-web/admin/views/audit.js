// 本文件负责审计日志页，展示操作记录。
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderPanel } from "../../shared/components/panel.js";
export function renderAudit(state, actions) {
    return renderPanel("审计", renderDataTable([
      { label: "时间", key: "created_at" },
      { label: "动作", key: "action" },
      { label: "人", render: (a) => a.user_id || "" },
      { label: "资源", render: (a) => a.resource || "" },
    ], state.audit, { showCount: true, label: "审计记录" }));
  }
