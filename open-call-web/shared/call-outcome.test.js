import test from "node:test";
import assert from "node:assert/strict";
import {
  applyCallEnded,
  isStaleCallMediaError,
  notifyCallFailure,
  outboundProgressLabel,
  recentCallResultLabel,
  shouldPromptWrapUp,
  userFacingCallEndMessage,
  userFacingFailureMessage,
} from "./call-outcome.js";

test("recentCallResultLabel uses switch result when present", () => {
  assert.equal(recentCallResultLabel({ result: "failed", hadActiveCall: true }), "失败");
  assert.equal(recentCallResultLabel({ result: "answered", hadActiveCall: true, elapsed: 0 }), "已接通");
});

test("recentCallResultLabel does not treat failed outbound as answered", () => {
  assert.equal(recentCallResultLabel({ hadActiveCall: true, elapsed: 0 }), "失败");
  assert.equal(recentCallResultLabel({ hadIncoming: true }), "未接听");
});

test("userFacingCallEndMessage prefers server message and end_message", () => {
  assert.equal(userFacingCallEndMessage({ message: "SIP 网关模组未注册", result: "failed" }), "SIP 网关模组未注册");
  assert.equal(userFacingCallEndMessage({ end_message: "出局失败", result: "failed" }), "出局失败");
});

test("userFacingFailureMessage covers leg.failed", () => {
  assert.equal(userFacingFailureMessage({ message: "对端忙（486）", error_code: "SIP_BUSY" }), "对端忙（486）");
});

test("shouldPromptWrapUp only after real talk", () => {
  assert.equal(shouldPromptWrapUp({ result: "failed", elapsed: 0 }), false);
  assert.equal(shouldPromptWrapUp({ result: "answered", elapsed: 0 }), true);
  assert.equal(shouldPromptWrapUp({ elapsed: 5 }), true);
});

test("outboundProgressLabel", () => {
  assert.equal(outboundProgressLabel("dialing"), "正在出局拨号…");
});

test("isStaleCallMediaError ignores join after call ended", () => {
  assert.equal(
    isStaleCallMediaError(new Error("当前状态无法加入媒体"), { callEnded: true }),
    true,
  );
  assert.equal(
    isStaleCallMediaError(new Error("当前状态无法加入媒体"), { joinEpoch: 1, currentJoinEpoch: 2 }),
    true,
  );
});

test("applyCallEnded invokes callback", () => {
  let meta = null;
  applyCallEnded({ feedback: { fail() {} } }, { call_id: "c1", result: "failed", message: "x" }, (m) => {
    meta = m;
  });
  assert.equal(meta?.result, "failed");
});
