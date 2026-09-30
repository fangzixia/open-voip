// 话单、小结与导出。
import { apiFetch } from "./client.js";

export function listCdr() {
  return apiFetch("/api/v1/cdr?page=1&page_size=50");
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
