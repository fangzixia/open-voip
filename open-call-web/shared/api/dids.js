// DID 路由与配置发布。
import { apiFetch } from "./client.js";

export function listDids() {
  return apiFetch("/api/v1/dids");
}

export function upsertDid(body) {
  return apiFetch("/api/v1/dids", { method: "POST", body: JSON.stringify(body) });
}

export function publishSwitchConfig() {
  return apiFetch("/api/v1/config/publish", { method: "POST" });
}
