const makeId = () => {
  if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
  const bytes = new Uint8Array(16);
  globalThis.crypto?.getRandomValues?.(bytes);
  if (!bytes.some(Boolean)) {
    for (let i = 0; i < bytes.length; i += 1) bytes[i] = Math.floor(Math.random() * 256);
  }
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = [...bytes].map((value) => value.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
};

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
