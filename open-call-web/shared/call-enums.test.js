// 本文件验证通话状态和类型常量的关键行为。
import test from "node:test";
import assert from "node:assert/strict";
import { callResultLabel, sessionTypeLabel } from "./call-enums.js";

test("call result and session type enums use Chinese labels", () => {
  assert.equal(callResultLabel("answered"), "已接通");
  assert.equal(callResultLabel("abandoned"), "已放弃");
  assert.equal(callResultLabel("timeout"), "超时");
  assert.equal(sessionTypeLabel("audio"), "语音");
  assert.equal(sessionTypeLabel("video"), "视频");
  assert.equal(sessionTypeLabel("mixed"), "音视频");
});

test("unknown enum values remain visible for forward compatibility", () => {
  assert.equal(callResultLabel("new_result"), "new_result");
  assert.equal(sessionTypeLabel("new_media"), "new_media");
  assert.equal(callResultLabel(""), "—");
});
