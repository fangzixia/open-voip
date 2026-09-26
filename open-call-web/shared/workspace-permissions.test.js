import test from "node:test";
import assert from "node:assert/strict";
import { availableViews } from "./workspace-permissions.js";

test("workspace navigation follows effective permissions and agent profile", () => {
  assert.deepEqual(availableViews({ agent_id: "a", permissions: ["agents.self", "agents.read", "queues.read"] }), { admin: false, agent: true });
  assert.deepEqual(availableViews({ agent_id: "a", permissions: ["agents.self", "users.read"] }), { admin: true, agent: true });
  assert.deepEqual(availableViews({ agent_id: "", permissions: ["roles.read"] }), { admin: true, agent: false });
  assert.deepEqual(availableViews({ agent_id: "", permissions: [] }), { admin: false, agent: false });
});
