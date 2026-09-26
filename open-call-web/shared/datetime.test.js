// 本文件验证datetime的关键行为。
import test from "node:test";
import assert from "node:assert/strict";
import { formatDate, formatDateTime } from "./datetime.js";

test("timestamp display uses the shared UTC format", () => {
  assert.equal(formatDateTime("2026-09-25T18:12:35+08:00"), "2026-09-25 10:12:35");
  assert.equal(formatDateTime("2026-09-25 10:12:35"), "2026-09-25 10:12:35");
  assert.equal(formatDateTime("invalid"), "");
});

test("date-only values use the UTC calendar date", () => {
  assert.equal(formatDate("2026-09-25T18:12:35+08:00"), "2026-09-25");
  assert.equal(formatDate("2026-09-25"), "2026-09-25");
  assert.equal(formatDate("2026-02-30"), "");
  assert.equal(formatDate("invalid"), "");
});
