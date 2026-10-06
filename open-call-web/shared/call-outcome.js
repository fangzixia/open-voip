// 通话结束与失败：与 Switch 事件 / CallView 对齐。
import { callResultLabel } from "./call-enums.js";

/**
 * @param {{ result?: string, elapsed?: number, hadActiveCall?: boolean, hadIncoming?: boolean }} opts
 */
export function recentCallResultLabel(opts) {
  const { result, elapsed = 0, hadActiveCall = false, hadIncoming = false } = opts;
  if (result) return callResultLabel(result);
  if (hadActiveCall && elapsed > 0) return callResultLabel("answered");
  if (hadActiveCall) return callResultLabel("failed");
  if (hadIncoming) return callResultLabel("no_answer");
  return "—";
}

/** @param {{ message?: string, result?: string, reason?: string, end_message?: string }} payload */
export function userFacingCallEndMessage(payload) {
  if (!payload) return "";
  const msg = payload.message || payload.end_message;
  if (msg) return String(msg);
  if (payload.result && payload.result !== "answered") {
    return callResultLabel(payload.result);
  }
  if (payload.reason === "error") return "通话异常结束";
  return "";
}

/** @param {{ message?: string, error?: string, error_code?: string, result?: string }} payload */
export function userFacingFailureMessage(payload) {
  if (!payload) return "";
  if (payload.message) return String(payload.message);
  if (payload.error) return String(payload.error);
  if (payload.result && payload.result !== "answered") {
    return callResultLabel(payload.result);
  }
  return "";
}

/**
 * @param {{ feedback?: { fail: (m: string) => void, liveNotice?: (m: string) => void } }} host
 * @param {Record<string, unknown>} payload
 * @param {{ toast?: boolean }} [opts]
 */
export function notifyCallFailure(host, payload, opts = {}) {
  const notice = userFacingFailureMessage(payload);
  if (!notice || !host?.feedback) return;
  if (opts.toast !== false && host.feedback.fail) {
    host.feedback.fail(notice);
  } else if (host.feedback.liveNotice) {
    host.feedback.liveNotice(notice);
  }
}

/**
 * @param {{ feedback?: { fail: (m: string) => void } }} host
 * @param {Record<string, unknown>} payload
 * @param {(meta: Record<string, unknown>) => void} onEndLocal
 */
export function applyCallEnded(host, payload, onEndLocal) {
  const notice = userFacingCallEndMessage(payload);
  if (notice && payload?.result !== "answered" && host?.feedback?.fail) {
    host.feedback.fail(notice);
  }
  onEndLocal(payload || {});
}

/** @param {{ result?: string, elapsed?: number }} opts */
export function shouldPromptWrapUp(opts) {
  const { result, elapsed = 0 } = opts;
  return result === "answered" || elapsed > 0;
}

/** @param {string} [phase] */
export function outboundProgressLabel(phase) {
  switch (phase) {
    case "dialing":
      return "正在出局拨号…";
    case "connected":
      return "对端已应答";
    case "failed":
      return "出局失败";
    default:
      return "";
  }
}
