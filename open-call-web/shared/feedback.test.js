import test from "node:test";
import assert from "node:assert/strict";
import { FeedbackController, currentFeedbackEpoch, installFeedbackEpochReader, runFeedbackAction } from "./feedback.js";

function mockHost() {
  const host = {
    error: "",
    notice: "",
    controllers: [],
    addController(c) { this.controllers.push(c); },
  };
  return host;
}

test("begin 递增 epoch 并清空横幅", () => {
  const host = mockHost();
  const fb = new FeedbackController(host);
  host.error = "旧错";
  host.notice = "旧提示";
  const e1 = fb.begin();
  assert.equal(e1, 1);
  assert.equal(host.error, "");
  assert.equal(host.notice, "");
  assert.equal(currentFeedbackEpoch(), 1);
});

test("过期 epoch 的 fail/ok 被忽略", () => {
  const host = mockHost();
  const fb = new FeedbackController(host);
  const old = fb.begin();
  fb.begin();
  assert.equal(fb.fail("不应出现", old), false);
  assert.equal(host.error, "");
  assert.equal(fb.ok("也不应出现", old), false);
  assert.equal(host.notice, "");
  assert.equal(fb.fail("当前错"), true);
  assert.equal(host.error, "当前错");
});

test("runFeedbackAction 成功与失败", async () => {
  const host = mockHost();
  const fb = new FeedbackController(host);
  const value = await runFeedbackAction(fb, async () => 42);
  assert.equal(value, 42);
  await runFeedbackAction(fb, async () => { throw new Error("操作失败"); });
  assert.equal(host.error, "操作失败");
});

test("断开后 epoch reader 复位", () => {
  const host = mockHost();
  const fb = new FeedbackController(host);
  fb.begin();
  assert.equal(currentFeedbackEpoch(), 1);
  fb.hostDisconnected();
  assert.equal(currentFeedbackEpoch(), 0);
  installFeedbackEpochReader(() => 0);
});
