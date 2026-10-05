import { createId, getCallContext } from "./call-context.js";
import { request } from "./http-client.js";

/** @type {Map<string, number>} */
const callVersionById = new Map();

export function setCallVersion(version, callId) {
  const cid = callId || getCallContext().call_id;
  if (!cid) return;
  callVersionById.set(cid, Number(version) > 0 ? Number(version) : 0);
}

export function getCallVersion(callId) {
  const cid = callId || getCallContext().call_id;
  if (!cid) return 0;
  return callVersionById.get(cid) || 0;
}

export function clearCallVersion(callId) {
  const cid = callId || getCallContext().call_id;
  if (cid) callVersionById.delete(cid);
}

export function callCommandBody(extra = {}) {
  const body = { ...extra };
  const v = getCallVersion();
  if (v > 0) body.expected_version = v;
  return body;
}

export function callCommandHeaders(action, callId) {
  const ctx = getCallContext();
  const cid = callId || ctx.call_id || "call";
  const key = `${action}-${cid}-v${getCallVersion(cid) || 0}`;
  return { "Idempotency-Key": key };
}

/** 通话写操作：附带乐观锁与幂等键；版本冲突时自动刷新一次并重试。 */
export async function callMutate(path, { method = "POST", action = "cmd", callId = "", body = {}, ...rest } = {}) {
  const cid = callId || getCallContext().call_id || "call";
  const run = () =>
    request(path, {
      method,
      body: JSON.stringify(callCommandBody(body)),
      headers: callCommandHeaders(action, cid),
      ...rest,
    });
  try {
    const data = await run();
    if (data?.version != null) setCallVersion(data.version, cid);
    return data;
  } catch (error) {
    const latest = error?.body?.data;
    if (error?.code === "VERSION_MISMATCH" && latest?.version != null) {
      setCallVersion(latest.version, cid);
      return run();
    }
    throw error;
  }
}

export function newIdempotencySuffix() {
  return createId();
}
