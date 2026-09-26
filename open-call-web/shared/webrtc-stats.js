// 本文件负责WebRTC 媒体统计采集与指标换算。
const counters = new WeakMap();

const values = (report) => {
  if (typeof report?.values === "function") return [...report.values()];
  const out = [];
  report?.forEach?.((value) => out.push(value));
  return out;
};

export function extractWebRTCStats(report, previous = {}, elapsedMs = 10000) {
  const rows = values(report);
  const selectedPair = rows.find((row) => row.type === "candidate-pair" && row.selected)
    || rows.find((row) => row.type === "candidate-pair" && row.nominated && row.state === "succeeded");
  const byId = new Map(rows.map((row) => [row.id, row]));
  const localCandidate = byId.get(selectedPair?.localCandidateId);
  const remoteCandidate = byId.get(selectedPair?.remoteCandidateId);
  const media = rows.filter((row) => row.type === "inbound-rtp" || row.type === "outbound-rtp");
  const totals = media.reduce((sum, row) => {
    sum.sent_bytes += Number(row.bytesSent || 0);
    sum.received_bytes += Number(row.bytesReceived || 0);
    sum.packets += Number(row.packetsSent || row.packetsReceived || 0);
    sum.packets_lost += Number(row.packetsLost || 0);
    if (Number.isFinite(row.jitter)) sum.jitter_ms = Math.max(sum.jitter_ms, row.jitter * 1000);
    if (Number.isFinite(row.framesPerSecond)) sum.fps = Math.max(sum.fps, row.framesPerSecond);
    return sum;
  }, { sent_bytes: 0, received_bytes: 0, packets: 0, packets_lost: 0, jitter_ms: 0, fps: 0 });
  const byteDelta = Math.max(0,
    totals.sent_bytes + totals.received_bytes
      - Number(previous.sent_bytes || 0) - Number(previous.received_bytes || 0));
  return {
    ...totals,
    bitrate_bps: elapsedMs > 0 ? Math.round((byteDelta * 8 * 1000) / elapsedMs) : 0,
    rtt_ms: Number.isFinite(selectedPair?.currentRoundTripTime) ? selectedPair.currentRoundTripTime * 1000 : 0,
    local_candidate_type: localCandidate?.candidateType || "",
    remote_candidate_type: remoteCandidate?.candidateType || "",
  };
}

export function startWebRTCStats(pc, report, intervalMs = 10000) {
  let previous = {};
  let previousAt = Date.now();
  const collect = async () => {
    try {
      const now = Date.now();
      const stats = extractWebRTCStats(await pc.getStats(), previous, now - previousAt);
      previous = stats;
      previousAt = now;
      report(stats);
    } catch (error) {
      report({ phase: "getStats", error_name: error?.name || "Error" });
    }
  };
  const timer = setInterval(collect, intervalMs);
  counters.set(pc, timer);
  return () => {
    clearInterval(timer);
    counters.delete(pc);
  };
}

export function stopWebRTCStats(pc) {
  const timer = counters.get(pc);
  if (timer) clearInterval(timer);
  counters.delete(pc);
}
