// 本文件负责审计日志页，展示操作记录。
import { renderDataTable } from "../../shared/components/data-table.js";
import { renderPageCrud } from "../../shared/components/page-layout.js";

export function renderAudit(state) {
  return renderPageCrud({
    sections: [{
      title: "审计记录",
      list: renderDataTable([
        { label: "时间", key: "created_at" },
        { label: "动作", key: "action" },
        { label: "人", render: (a) => a.user_id || "" },
        { label: "资源", render: (a) => a.resource || "" },
      ], state.audit, { showCount: true, label: "审计记录", emptyMessage: "暂无审计记录" }),
    }],
  });
}
