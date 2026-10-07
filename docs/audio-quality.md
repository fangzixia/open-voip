# 通话音质档位（窄带 PSTN）

产品收敛为 **无视频、SIP/WebRTC 混音房间均为 G.711 @ 8 kHz（PCMA/PCMU，ptime 20 ms）**。

| 档位 | 配置值 | SIP | WebRTC 坐席/AI |
|------|--------|-----|----------------|
| 标准窄带 | `narrowband`（唯一推荐） | PCMA/PCMU @ 8 kHz | PCMU/8000 |

`wideband` / `hd_webrtc` 在媒体层不再扩展能力；队列配置应使用 `narrowband`。

## 录音（FS 式 tap）

| 文件 | 内容 |
|------|------|
| 主录音 `callId-recId.wav` | **客户 leg** 听到的混音（`dispatchMixTick` 的 `mixed`，与实时听感一致） |
| 分轨 `...-leg-<id>.wav` | 该 leg **上行 decode 后**顺序 PCM |
| 采样率 | **8000 Hz**（`record_engine=tap`，`record_sample_rate_hz=8000`） |

IVR 阶段若启用 `gate_inbound_until_prompt`：主录仅写入出站提示音；坐席接通后 control 调用 `SetRecordingMixInbound(true)` 再收录上行分轨。

## 日志字段（排查电话侧编码）

| 事件 | 组件 | 关键字段 |
|------|------|----------|
| `audio.profile` | media | `audio_profile`（应为 narrowband） |
| `codec.negotiated` | sip_rtp | `codec_negotiated`、`payload_type`、`sample_rate_hz` |
| `rtp.summary` | sip_rtp | `sequence_gaps`、`out_of_order` |
| `rtp.ptime_mismatch` | media | 需 `media.log_rtp_ptime_mismatch: true` |
| `recording.opened` | media | `record_sample_rate_hz=8000`、`record_engine=tap` |
| `quality.sample` | webrtc | `stats_summary`、`rtt_ms` |

## 四路采集对照

同一 `call_id`：SIP 分轨、坐席上行分轨、听客户侧 mixed（主录）、`rtp.summary`。

## ECS 发版检查

```bash
ssh open-voip "grep '<call_id>' /opt/open-switch/logs/trace.jsonl | grep -E 'recording.opened|rtp.summary|codec.negotiated'"
# 期望：record_sample_rate_hz=8000，record_engine=tap
```
