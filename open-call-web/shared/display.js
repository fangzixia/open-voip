// 本文件负责界面展示值的格式化。
const AGENT_STATE_LABELS = {
  idle: "空闲",
  busy: "示忙",
  ringing: "振铃",
  on_call: "通话中",
  acw: "事后处理",
  offline: "未签入",
};

const AGENT_STATE_TONES = {
  idle: "ok",
  busy: "warn",
  ringing: "info",
  on_call: "info",
  acw: "warn",
  offline: "muted",
};

export function formatDuration(seconds) {
  const total = Math.max(0, Number(seconds) || 0);
  const minutes = Math.floor(total / 60);
  const rest = Math.floor(total % 60);
  return `${String(minutes).padStart(2, "0")}:${String(rest).padStart(2, "0")}`;
}

export function agentStateLabel(state) {
  return AGENT_STATE_LABELS[state] || state || "未签入";
}

export function agentStateTone(state) {
  return AGENT_STATE_TONES[state] || "muted";
}
