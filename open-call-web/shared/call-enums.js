const CALL_RESULT_LABELS = {
  answered: "已接通",
  abandoned: "已放弃",
  no_answer: "未接听",
  failed: "失败",
  timeout: "超时",
  queued: "排队中",
};

const SESSION_TYPE_LABELS = {
  audio: "语音",
  video: "视频",
  mixed: "音视频",
};

export function callResultLabel(value) {
  return CALL_RESULT_LABELS[value] || value || "—";
}

export function sessionTypeLabel(value) {
  return SESSION_TYPE_LABELS[value] || value || "—";
}
