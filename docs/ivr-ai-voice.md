# AI 语音 IVR 与统一 WebRTC 媒体

## 架构

- **欢迎语**：Switch IVR `play` 节点 + `InjectAudio`（单向）。
- **AI 对话 / 语音通知**：open-call 内嵌 **Go Worker**（`internal/aibot`）以虚拟坐席 **`JoinWebRTC`** 进媒体；Switch 无 AI 专用分支。
- **语音通知**：不再在 Switch 对 PSTN `InjectAudio`；Worker 桥接 agent↔PSTN 后播放 WAV。

## IVR 流程示例

1. 管理端 `GET /api/v1/aibot/ivr-template?queue_id=<AI队列ID>` 获取草稿。
2. 为 `welcome` 节点选择 WAV 欢迎语，发布 IVR。
3. DID `target_type=ivr` 绑定该流程，或队列 `ivr_flow_id` 指向该流程。
4. 节点链：`play`（欢迎）→ `route_queue`（AI 专用队列）。

## open-call 配置（`aibot`）

见 [open-call/deploy/config.example.yml](../open-call/deploy/config.example.yml)。

- `agent_id`：虚拟坐席 ID（WebRTC），须签入 `queue_ids`。
- `queue_ids`：AI 专用队列；仅该 Worker 签入，避免与真人抢单。
- `openai_realtime.base_url`：**必填**（启用 AI 队列时），指向兼容 Realtime WebSocket 的网关；系统**不会**默认 `api.openai.com`。
- 语音通知：`POST .../voice-notifications` 的 **agent_id 必须与 `aibot.agent_id` 相同**。

## 队列提示词

`PUT /api/v1/aibot/queues/{queueId}/profile` 保存 `system_prompt`（可与全局 `aibot.system_prompt` 配合；Worker 当前使用配置内全局 prompt）。

## 事件

- AI 用量：`ai.session.completed` webhook（`input_audio_ms` / `output_audio_ms`）。
- 语音通知就绪：`call.outbound_progress` phase `media_ready`。

## 联调

1. 启用 `aibot.enabled`，配置 Realtime 密钥与虚拟坐席账号。
2. 先测语音通知（单向），再测呼入 IVR → AI 队列。
3. 按 [audio-quality.md](./audio-quality.md) 查看 `call_id` 下 `codec.negotiated` / `prompt.play` 日志。
