// 本文件验证访客邀请信息解析与校验的关键行为。
import test from "node:test";
import assert from "node:assert/strict";
import { parseGuestInvite } from "./guest-invite.js";

test("guest invite parses video token and safely defaults media", () => {
  assert.deepEqual(parseGuestInvite("?token=abc&media=video"), { token: "abc", media: "video" });
  assert.deepEqual(parseGuestInvite("?token=abc&media=other"), { token: "abc", media: "audio" });
  assert.deepEqual(parseGuestInvite(""), { token: "", media: "audio" });
});
