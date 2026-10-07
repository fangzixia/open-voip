// 本文件验证WebRTC 媒体统计采集与指标换算的关键行为。
import test from "node:test";
import assert from "node:assert/strict";
import { extractWebRTCStats } from "./webrtc-stats.js";

test("extracts aggregate media quality and selected candidate types", () => {
  const report = new Map([
    ["in", { id: "in", type: "inbound-rtp", bytesReceived: 3000, packetsReceived: 30, packetsLost: 2, jitter: 0.012, framesPerSecond: 24 }],
    ["out", { id: "out", type: "outbound-rtp", bytesSent: 5000, packetsSent: 40, framesPerSecond: 30 }],
    ["pair", { id: "pair", type: "candidate-pair", selected: true, currentRoundTripTime: 0.08, localCandidateId: "local", remoteCandidateId: "remote" }],
    ["local", { id: "local", type: "local-candidate", candidateType: "relay" }],
    ["remote", { id: "remote", type: "remote-candidate", candidateType: "srflx" }],
  ]);
  const stats = extractWebRTCStats(report, { sent_bytes: 1000, received_bytes: 1000 }, 10000);
  assert.equal(stats.sent_bytes, 5000);
  assert.equal(stats.received_bytes, 3000);
  assert.equal(stats.packets, 70);
  assert.equal(stats.packets_lost, 2);
  assert.equal(stats.jitter_ms, 12);
  assert.equal(stats.fps, 30);
  assert.equal(stats.bitrate_bps, 4800);
  assert.equal(stats.rtt_ms, 80);
  assert.equal(stats.local_candidate_type, "relay");
  assert.equal(stats.remote_candidate_type, "srflx");
  assert.ok(Array.isArray(stats.inbound));
  assert.ok(Array.isArray(stats.outbound));
  assert.equal(typeof stats.browser_caps, "object");
});
