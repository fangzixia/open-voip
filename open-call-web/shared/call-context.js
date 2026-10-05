// 本文件负责通话上下文的构建与读取。
import { randomId as makeId } from "./random-id.js";

const context = {
  client_session_id: makeId(),
  trace_id: makeId(),
  call_id: "",
  leg_id: "",
  agent_id: "",
  queue_id: "",
  role: "",
};

export function createId() {
  return makeId();
}

export function getCallContext() {
  return { ...context };
}

export function setCallContext(next = {}) {
  for (const key of ["trace_id", "call_id", "leg_id", "agent_id", "queue_id", "role"]) {
    if (Object.hasOwn(next, key)) context[key] = next[key] == null ? "" : String(next[key]);
  }
  return getCallContext();
}

export function beginTrace(next = {}) {
  context.trace_id = makeId();
  return setCallContext(next);
}

export function clearCallContext() {
  context.call_id = "";
  context.leg_id = "";
  context.queue_id = "";
  return getCallContext();
}
