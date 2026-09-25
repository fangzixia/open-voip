import test from "node:test";
import assert from "node:assert/strict";
import { beginTrace, clearCallContext, getCallContext, setCallContext } from "./call-context.js";

test("call context keeps session identity and manages call fields", () => {
  const initial = getCallContext();
  assert.match(initial.client_session_id, /^[0-9a-f-]{36}$/);
  assert.match(initial.trace_id, /^[0-9a-f-]{36}$/);
  setCallContext({ call_id: "call-1", leg_id: "leg-1", agent_id: "agent-1", role: "agent" });
  assert.deepEqual(getCallContext(), { ...initial, call_id: "call-1", leg_id: "leg-1", agent_id: "agent-1", role: "agent" });
  const next = beginTrace({ role: "guest", agent_id: "" });
  assert.notEqual(next.trace_id, initial.trace_id);
  clearCallContext();
  assert.equal(getCallContext().call_id, "");
  assert.equal(getCallContext().leg_id, "");
});
