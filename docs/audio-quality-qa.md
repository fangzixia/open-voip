# 音质终端与听感 QA 清单

在服务端媒体时序修复后，若仍不清晰，按本清单排除终端与线路因素。

## 坐席 WebRTC

1. 记录 `track.getSettings()`（设备 ID、echoCancellation、noiseSuppression、autoGainControl）。
2. 固定耳机与嘴距，做 **浏览器 AGC 开/关** 各一通对照（勿叠加系统增强与浏览器增益）。
3. 查看 `quality.sample` 中 `stats_summary`：按 SSRC 的 `packets_lost`、`jitter`、`concealed_samples`。

## SIP 话机 / 模组

1. 单独听测主叫侧（不经过录音），确认是否为线路/终端底噪。
2. 核对模组/运营商 **ptime** 与 `rtp.ptime_mismatch` 日志。

## 降噪

仅当 **分轨 WAV** 显示某路持续底噪时，再评估该路单次降噪；避免对全混音做强处理以免损伤辅音。
