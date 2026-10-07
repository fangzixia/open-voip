# 外部媒体后端（FreeSWITCH / rtpengine）PoC 说明

open-switch 已通过 [`MediaBackend`](../open-switch/internal/layers/media/backend.go) 抽象 in-process 媒体；默认实现为 `inProcessBackend`（现有 Pion + sipgo + tap 录音）。

## 未来 FS 对接要点

| in-process | FreeSWITCH |
|------------|------------|
| `CreateSession` | ESL `originate` / 入站 dialplan |
| `BridgeLegs` | `uuid_bridge` |
| `StartRecording` | `uuid_record` / `record_session` |
| tap 主录/分轨 | 每 UUID 一条 record 或 conference `--record` |

控制面（Switch API、FSM、open-call）保持不变，仅替换 `Service.backend` 为 `fsBackend` 实现。

## PoC 步骤（未纳入默认构建）

1. 部署 FS，配置 `mod_sofia` 与 ESL。
2. 实现 `fsBackend.StartRecording` → `api uuid_record <uuid> start <path>`。
3. 用一条 Direct 呼叫验证 WAV 与 `recording.opened` 元数据回写。

当前生产仍使用 **tap 录音 @ 8 kHz**，见 [audio-quality.md](./audio-quality.md)。
