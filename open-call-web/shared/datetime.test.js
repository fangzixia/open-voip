// 本文件验证datetime的关键行为。
import test from "node:test";
import assert from "node:assert/strict";
import { formatDate, formatDateTime } from "./datetime.js";

test("timestamp display uses local wall clock (aligned with backend datetime)", () => {
  const instant = new Date("2026-09-25T18:12:35+08:00");
  assert.equal(formatDateTime("2026-09-25T18:12:35+08:00"), formatDateTime(instant));
  assert.equal(formatDateTime("2026-09-25 10:12:35"), "2026-09-25 10:12:35");
  assert.equal(formatDateTime("invalid"), "");
});

test("date-only values use local calendar date", () => {
  const instant = new Date("2026-09-25T18:12:35+08:00");
  assert.equal(formatDate("2026-09-25T18:12:35+08:00"), formatDate(instant));
  assert.equal(formatDate("2026-09-25"), "2026-09-25");
  assert.equal(formatDate("2026-02-30"), "");
  assert.equal(formatDate("invalid"), "");
});
