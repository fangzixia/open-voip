// 本文件验证身份管理操作的关键行为。
import test from "node:test";
import assert from "node:assert/strict";
import { createUserPayload, newIdentityUserDraft } from "./identity.js";

test("one user creation flow preserves custom roles and SIP agent settings", () => {
  const draft = { ...newIdentityUserDraft(), username: "alice", roles: ["agent", "reviewer"], extension: " 1001 ", terminal_type: "sip", video_capable: true };
  const payload = createUserPayload(draft, true);
  assert.deepEqual(payload.roles, ["agent", "reviewer"]);
  assert.equal("role" in payload, false);
  assert.equal(payload.extension, "1001");
  assert.equal(payload.sip_username, "1001");
  assert.equal(payload.video_capable, false);
});

test("user creation still works when role listing is unavailable", () => {
  const draft = { ...newIdentityUserDraft(), extension: "2002" };
  const payload = createUserPayload(draft, false);
  assert.equal(payload.role, "agent");
  assert.equal("roles" in payload, false);
  assert.equal(payload.terminal_type, "webrtc");
  assert.equal(payload.video_capable, true);
});
