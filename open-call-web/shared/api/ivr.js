// IVR 流程、版本与语音素材。
import { apiFetch } from "./client.js";

export function createIvrFlow(name, draft) {
  return apiFetch("/api/v1/ivr/flows", { method: "POST", body: JSON.stringify({ name, draft }) });
}

export function publishIvr(flowId) {
  return apiFetch(`/api/v1/ivr/flows/${flowId}/publish`, { method: "POST" });
}

export function listIvr() {
  return apiFetch("/api/v1/ivr/flows");
}

export function getIvrFlow(id) {
  return apiFetch(`/api/v1/ivr/flows/${encodeURIComponent(id)}`);
}

export function updateIvrFlow(id, name, draft) {
  return apiFetch(`/api/v1/ivr/flows/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify({ name, draft }),
  });
}

export function deleteIvrFlow(id) {
  return apiFetch(`/api/v1/ivr/flows/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export function listIvrVersions(id) {
  return apiFetch(`/api/v1/ivr/flows/${encodeURIComponent(id)}/versions`);
}

export function rollbackIvrFlow(id, version) {
  return apiFetch(`/api/v1/ivr/flows/${encodeURIComponent(id)}/rollback`, {
    method: "POST",
    body: JSON.stringify({ version }),
  });
}

export function listIvrAssets() {
  return apiFetch("/api/v1/ivr-assets");
}

export function uploadIvrAsset(file) {
  const body = new FormData();
  body.append("file", file);
  return apiFetch("/api/v1/ivr-assets", { method: "POST", body, timeoutMs: 120000 });
}

export function fetchIvrAsset(id) {
  return apiFetch(`/api/v1/ivr-assets/${encodeURIComponent(id)}`, {
    responseType: "blob",
    timeoutMs: 120000,
  });
}

export function listIvrTtsOptions() {
  return apiFetch("/api/v1/ivr-assets/tts-options");
}

export function synthesizeIvrAsset({ name, text, voice }) {
  const body = { name, text };
  if (voice) body.voice = voice;
  return apiFetch("/api/v1/ivr-assets/synthesize", {
    method: "POST",
    body: JSON.stringify(body),
    timeoutMs: 120000,
  });
}
