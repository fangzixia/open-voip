// 话单、小结与导出。
import { apiFetch } from "./client.js";

/** 服务端过滤 + 分页；返回 { items, page, page_size, total }。 */
export function listCdr({ page = 1, pageSize = 50, caller = "", result = "" } = {}) {
  const q = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
  if (caller) q.set("caller", caller);
  if (result) q.set("result", result);
  return apiFetch(`/api/v1/cdr?${q}`);
}

export function listWrapUps(callId) {
  const q = callId ? `?call_id=${encodeURIComponent(callId)}` : "";
  return apiFetch(`/api/v1/wrap-ups${q}`);
}

export async function downloadCdrCsv() {
  const range = new URLSearchParams({ from: "2020-01-01 00:00:00", to: "2099-01-01 00:00:00" });
  const blob = await apiFetch(`/api/v1/cdr/export.csv?${range}`, { responseType: "blob", timeoutMs: 120000 });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "cdr.csv";
  a.click();
  URL.revokeObjectURL(url);
}
