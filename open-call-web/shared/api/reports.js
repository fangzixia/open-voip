// 状态与报表。
import { formatDate, formatDateTime } from "../datetime.js";
import { apiFetch } from "./client.js";

export function fetchStatus() {
  return apiFetch("/api/v1/status", { method: "GET" });
}

export function fetchLiveReport() {
  return apiFetch("/api/v1/reports/live");
}

function todayDateRange() {
  const today = formatDate();
  return new URLSearchParams({ from: today, to: today });
}

/** 报表范围采用 UTC 当日零点及统一时间格式。 */
function todayReportRange() {
  const now = new Date();
  const start = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  const end = new Date(Math.max(now.getTime(), start.getTime() + 1));
  return new URLSearchParams({ from: formatDateTime(start), to: formatDateTime(end) });
}

export function fetchHistoricalReport() {
  return apiFetch(`/api/v1/reports/historical?${todayDateRange()}`);
}

export function fetchAgentUtil() {
  return apiFetch(`/api/v1/reports/agents?${todayReportRange()}`);
}
