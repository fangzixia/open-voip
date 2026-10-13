// 本文件负责录音管理页，展示文件和质检信息。
import { renderDataTable } from "../../shared/components/data-table.js";
import { recordingEntityColumns } from "../../shared/components/recording-table-cells.js";
import { renderPageCrud } from "../../shared/components/page-layout.js";
import { recordingSemanticsLabel, recordingStatusLabel } from "../../shared/recording-meta.js";

export function renderRecs(state, actions) {
  return renderPageCrud({
    sections: [{
      title: "录音列表",
      list: renderDataTable([
        { label: "ID", key: "id" },
        { label: "通话", key: "call_id" },
        { label: "录制内容", render: (r) => recordingSemanticsLabel(r) },
        { label: "状态", render: (r) => recordingStatusLabel(r) },
        ...recordingEntityColumns(state, actions),
      ], state.recs, { showCount: true, label: "录音列表", emptyMessage: "暂无录音" }),
    }],
  });
}
