// 本文件验证observability的关键行为。
import test from "node:test";
import assert from "node:assert/strict";

globalThis.window = { location: { origin: "http://localhost:5173" }, addEventListener() {} };
globalThis.location = window.location;
const { EventReporter, sanitizeDetails } = await import("./observability.js");

test("sanitizer only keeps bounded non-sensitive telemetry", () => {
  assert.deepEqual(sanitizeDetails({
    phase: "created",
    token: "secret",
    sdp: "v=0",
    candidate: "candidate:full",
    device_label: "My microphone",
    local_candidate_type: "relay",
    unknown: "drop",
  }), { phase: "created", local_candidate_type: "relay" });
});

test("reporter batches events and retries a failed batch", async () => {
  const calls = [];
  const timers = [];
  const reporter = new EventReporter({
    fetchImpl: async (_url, init) => {
      calls.push(JSON.parse(init.body));
      return { ok: calls.length > 1, status: 503 };
    },
    setTimer: (fn, delay) => { timers.push({ fn, delay }); return timers.length; },
    clearTimer() {},
    getToken: () => "test-token",
  });
  reporter.report("ws.open", { token: "never" });
  assert.equal(await reporter.flush(), false);
  assert.equal(reporter.queue.length, 1);
  assert.equal(timers.at(-1).delay, 1000);
  assert.equal(await reporter.flush(), true);
  assert.equal(reporter.queue.length, 0);
  assert.equal(calls[0].events[0].fields.token, undefined);
  assert.equal(calls[0].events[0].type, "ws.open");
});
