# 通话音质档位

平台支持多档音质，由队列 `audio_profile` 与 SIP `prefer_wideband` 共同决定协商结果。

| 档位 | 配置值 | SIP | WebRTC |
|------|--------|-----|--------|
| 标准窄带 | `narrowband`（默认） | G.711 PCMA/PCMU @ 8 kHz | Opus（坐席腿） |
| 高清宽带 | `wideband` | 对端支持时 G.722 @ 16 kHz | Opus |
| 全高清 | `hd_webrtc` | 宽带 SIP（若支持） | 访客 WebRTC Opus 48 kHz |

IVR 素材上传会自动生成 8 kHz 电话母带与可选 48 kHz HD 副本（`{id}_48k.wav`）。

## 日志字段（排查电话侧编码）

| 事件 | 组件 | 关键字段 |
|------|------|----------|
| `audio.profile` | media | `audio_profile`、`prefer_wideband`、`prefer_opus` |
| `codec.negotiated` | sip_rtp | `codec_negotiated`、`payload_type`、`sample_rate_hz`、`offer_codecs`、`audio_profile` |
| `rtp.summary` | sip_rtp | 挂断汇总 + `codec_negotiated`、`sample_rate_hz` |
| `prompt.play` | media | IVR：`codec_negotiated`、`prompt_late_ms`、`frames_sent` |
| `room.created` | media | `audio_profile`、`sip_prefer_wideband` |
| `recording.opened` | media | `record_sample_rate_hz`（8k / 16k / 48k） |

结构化追踪与 `slog` 文本日志会同时输出；按 `call_id` 检索即可。
