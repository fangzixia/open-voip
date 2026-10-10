import test from "node:test";
import assert from "node:assert/strict";
import { renderRecs } from "../views/recordings.js";

// Inspect nested Lit templates without executing callbacks or requiring a DOM.
function text(value) {
  if (Array.isArray(value)) return value.map(text).join("");
  if (value?.strings && value?.values) return value.strings.map((part, i) => part + text(value.values[i])).join("");
  return typeof value === "string" || typeof value === "number" ? String(value) : "";
}

test("partial recordings expose failure and their actual sample duration", () => {
  const output = text(renderRecs({ recs: [{ id: "partial", recording_semantics: "conversation_mono_v1", status: "failed", failure_reason: "磁盘写入失败", duration_samples: 9600, sample_rate_hz: 8000, format: "wav" }] }, {}));
  assert.match(output, /完整通话（单声道）/);
  assert.match(output, /失败 · 磁盘写入失败/);
  assert.match(output, /1\.20 秒/);
  assert.doesNotMatch(output, /已完成/);
});

test("legacy content and ongoing or unknown status remain distinguishable", () => {
  const output = text(renderRecs({ recs: [{ id: "old", recording_semantics: "legacy", status: "completed" }, { id: "active", status: "recording" }, { id: "unknown" }] }, {}));
  assert.match(output, /历史录音/);
  assert.match(output, /已完成/);
  assert.match(output, /录制中/);
  assert.match(output, /状态未知/);
});
