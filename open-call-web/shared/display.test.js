// 本文件验证界面展示值的格式化的关键行为。
import test from "node:test";
import assert from "node:assert/strict";
import { agentStateLabel, agentStateTone, formatDuration } from "./display.js";

test("shared duration formatting is stable", () => {
  assert.equal(formatDuration(0), "00:00");
  assert.equal(formatDuration(125), "02:05");
  assert.equal(formatDuration(-3), "00:00");
});

test("agent states share labels and semantic tones", () => {
  assert.equal(agentStateLabel("on_call"), "通话中");
  assert.equal(agentStateTone("idle"), "ok");
  assert.equal(agentStateLabel("future"), "future");
});
