import { createId, getCallContext } from "./call-context.js";
import { request } from "./http-client.js";

let callVersion = 0;

export function setCallVersion(version) {
  callVersion = Number(version) > 0 ? Number(version) : 0;
}

export function getCallVersion() {
  return callVersion;
}

export function callCommandBody(extra = {}) {
  const body = { ...extra };
  if (callVersion > 0) body.expected_version = callVersion;
  return body;
}

export function callCommandHeaders(action, callId) {
  const ctx = getCallContext();
  const key = `${action}-${callId || ctx.call_id || "call"}-v${callVersion || 0}`;
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
    return data;
  } catch (error) {
    const latest = error?.body?.data;
    if (error?.code === "VERSION_MISMATCH" && latest?.version != null) {
      setCallVersion(latest.version);
      return run();
    }
    throw error;
  }
}

export function newIdempotencySuffix() {
  return createId();
}
