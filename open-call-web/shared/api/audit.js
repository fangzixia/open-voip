// 审计日志。
import { apiFetch } from "./client.js";

export function listAudit() {
  return apiFetch("/api/v1/audit/logs?page=1&page_size=50");
}
