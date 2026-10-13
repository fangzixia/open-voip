import test from "node:test";
import assert from "node:assert/strict";
import { renderRecordingPlayerModal } from "./recording-player-modal.js";

function text(value) {
  if (Array.isArray(value)) return value.map(text).join("");
  if (value?.strings && value?.values) return value.strings.map((part, i) => part + text(value.values[i])).join("");
  return typeof value === "string" || typeof value === "number" ? String(value) : "";
}

test("closed modal renders nothing", () => {
  assert.equal(text(renderRecordingPlayerModal({ open: false, playUrl: "blob:x" })), "");
});

test("open modal with audio includes controls", () => {
  const output = text(renderRecordingPlayerModal({
    open: true,
    playUrl: "blob:test",
    playType: "audio/ogg",
  }));
  assert.match(output, /controls/);
  assert.match(output, /关闭/);
});
