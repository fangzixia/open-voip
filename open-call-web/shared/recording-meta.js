// 录音列表展示用的格式、大小、时长标签。

const EMPTY = "—";

/** 是否按视频在模态框中播放。 */
export function recordingIsVideo(r) {
  if (!r) return false;
  if (r.media_type === "video_composite") {
    const fmt = (r.format || "").toLowerCase();
    return ["mp4", "webm"].includes(fmt);
  }
  return (r.format || "").toLowerCase() === "mp4" || (r.format || "").toLowerCase() === "webm";
}

/** 列「录音格式」。 */
export function recordingFormatLabel(r) {
  if (!r) return EMPTY;
  const fmt = (r.format || "").trim();
  if (r.media_type === "video_composite") {
    if (["mp4", "webm"].includes(fmt.toLowerCase())) return fmt.toUpperCase();
    return "旧版分离";
  }
  return (fmt || "ogg").toUpperCase();
}

/** 字节 → KB 字符串。 */
export function formatFileSizeKb(bytes) {
  if (bytes == null || bytes < 0 || Number.isNaN(Number(bytes))) return EMPTY;
  if (bytes === 0) return "0 KB";
  const kb = bytes / 1024;
  if (kb >= 100) return `${Math.round(kb)} KB`;
  return `${kb.toFixed(1)} KB`;
}

export function recordingSizeKbLabel(r) {
  if (!r) return EMPTY;
  return formatFileSizeKb(r.file_size);
}

function pad2(n) {
  return String(n).padStart(2, "0");
}

/** 列「录音时长」。 */
export function recordingDurationLabel(r) {
  if (!r) return EMPTY;
  const rate = r.sample_rate_hz;
  const samples = r.duration_samples;
  if (rate > 0 && samples != null && samples >= 0) {
    const sec = samples / rate;
    if (sec >= 60) {
      const m = Math.floor(sec / 60);
      const s = Math.floor(sec % 60);
      return `${pad2(m)}:${pad2(s)}`;
    }
    return `${sec.toFixed(2)} 秒`;
  }
  const start = r.started_at ? Date.parse(r.started_at) : NaN;
  const end = r.ended_at ? Date.parse(r.ended_at) : NaN;
  if (!Number.isNaN(start) && !Number.isNaN(end) && end >= start) {
    const sec = (end - start) / 1000;
    if (sec >= 60) {
      const m = Math.floor(sec / 60);
      const s = Math.floor(sec % 60);
      return `${pad2(m)}:${pad2(s)}`;
    }
    return `${sec.toFixed(2)} 秒`;
  }
  return EMPTY;
}

export function recordingSemanticsLabel(r) {
  if (!r) return EMPTY;
  return r.recording_semantics === "conversation_mono_v1" ? "完整通话（单声道）" : "历史录音";
}

export function recordingStatusLabel(r) {
  if (!r?.status) return "状态未知";
  if (r.status === "failed") return `失败 · ${r.failure_reason || "录音不完整"}`;
  if (r.status === "recording") return "录制中";
  if (r.status === "completed") return "已完成";
  return "状态未知";
}

export function recordingPlayDisabled(r) {
  return r?.status === "recording";
}

export function recordingPlayDisabledTitle(r) {
  if (r?.status === "failed") return r.failure_reason || "录音不完整";
  if (r?.status === "recording") return "录制尚未完成";
  return "";
}
