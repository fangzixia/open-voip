import assert from "node:assert/strict";
import { describe, it, beforeEach, mock } from "node:test";

describe("call-mutation", () => {
  beforeEach(() => {
    mock.reset();
  });

  it("adds expected_version and idempotency headers", async () => {
    const { setCallVersion, callCommandBody, callCommandHeaders } = await import("./call-mutation.js");
    setCallVersion(3);
    assert.deepEqual(callCommandBody({ on: true }), { on: true, expected_version: 3 });
    assert.equal(callCommandHeaders("hold", "c1")["Idempotency-Key"], "hold-c1-v3");
  });
});
