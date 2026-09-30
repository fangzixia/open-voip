// 录音列表与下载。
import { apiFetch } from "./client.js";

export function listRecordings(callId) {
  const q = callId ? `?call_id=${callId}` : "";
  return apiFetch(`/api/v1/recordings${q}`);
}

export function fetchRecordingBlob(id, format = "") {
  const query = format ? `?format=${encodeURIComponent(format)}` : "";
  return apiFetch(`/api/v1/recordings/${id}/download${query}`, {
    responseType: "blob",
    timeoutMs: 600000,
  });
}

function extFromType(type) {
  if (!type) return ".ogg";
  if (type.includes("webm")) return ".webm";
  if (type.includes("mp4")) return ".mp4";
  if (type.includes("ogg")) return ".ogg";
  if (type.includes("wav")) return ".wav";
  if (type.includes("ivf")) return ".ivf";
  return ".bin";
}

export async function downloadRecording(id, format = "") {
  const blob = await fetchRecordingBlob(id, format);
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `${id}${extFromType(blob.type)}`;
  a.click();
  URL.revokeObjectURL(url);
}
