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
  assert.deepEqual(extractWebRTCStats(report, { sent_bytes: 1000, received_bytes: 1000 }, 10000), {
    sent_bytes: 5000,
    received_bytes: 3000,
    packets: 70,
    packets_lost: 2,
    jitter_ms: 12,
    fps: 30,
    bitrate_bps: 4800,
    rtt_ms: 80,
    local_candidate_type: "relay",
    remote_candidate_type: "srflx",
  });
});
