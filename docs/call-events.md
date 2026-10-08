# 呼叫事件契约（Switch → open-call → WebSocket）

Switch 通过 `integration.events_callback_url` 推送事件；open-call 投影后写入 outbox 并广播到坐席/访客 WebSocket。

## call.ended（必填语义）

| 字段 | 类型 | 说明 |
|------|------|------|
| `call_id` | string | 通话 ID |
| `reason` | string | `normal` / `error` / `timeout` / `abandon` / `transfer` |
| `result` | string | 与 CDR 一致：`answered` / `failed` / `abandoned` |
| `message` | string | 面向用户的中文说明；`result != answered` 时应提供 |
| `error_code` | string | 可选机器码，如 `SIP_DISABLED`、`IVR_PROMPT_FAILED` |

UI **不得**用本地 `call` 对象是否存在推断「已接通」，必须以 `result` / `message` 为准。

## 过程性失败（不替代 call.ended）

| 类型 | 典型场景 | 载荷 |
|------|----------|------|
| `command.failed` | IVR 跳数超限等 | `message`, `error_code`, `reason`, `call_id` |
| `recording.failed` | 录音启动失败 | `message`, `call_id` |
| `leg.failed` | Direct SIP 拨号失败 | `message`, `error_code`, `leg_id`, `call_id` |
| `call.outbound_progress` | PSTN 出局阶段 | `phase`: `dialing` / `connected` / `failed`, `message?` |
| `call.device_failed` | SIP 坐席设备不可达 | `call_id`（配合后续 `call.ended`） |

## ivr.input_collected（通用交互结果）

Switch 在 `collect_input` 收到允许的单个 DTMF 后发布：`call_id`、`agent_id`（有历史接听坐席时）、`flow_id`、`flow_version`、`node_id`、`result_key`、`input`。输入保留字符串形式，包括 `0`、`*`、`#`；不包含 `score`。超时和不允许的按键不产生本事件。

open-call 仅将 `result_key=csat` 且 `input` 为单个 `1–5` 的事件解释为满意度并投影到 `oc_call_csat`；其他结果继续作为通用事件。业务流程由 open-call 编译，Switch 不解释结果标识。旧 `call.csat_scored` 已移除，升级前应排空旧事件并重新发布流程，见[升级步骤](open-switch对接说明.md)。

## HTTP CallView 扩展

`GET /switch/v1/internal/calls/{id}` 与 BFF `getCall` 增加：

- `result` — 与 CDR/ended 一致（ended 后可从话单推断）
- `end_message` — 与 WS `message` 对齐
- `error_code`
- `pstn_dial_state` — 外呼 PSTN：`pending` / `dialing` / `connected` / `failed`（进行中通话）
- `post_call_ivr_flow_id` — 当前队列快照的后续流程绑定，由 open-call 选择其业务用途

## UI 责任矩阵

| 端 | 必订阅 |
|----|--------|
| 坐席 | `call.ended`, `call.outbound_progress`, `recording.failed`, `command.failed` |
| 访客 | `call.ended`, `recording.failed`, `command.failed` |
| 管理 runtime | `getCall` 展示 `result` / `end_message`；Direct 拨号轮询 CallView |

## ECS 验收清单

1. 网关未注册外呼：`result=failed`，`message` 含网关/SIP 说明，坐席不显示「已接通」。
2. SIP 486/480：message 含忙/不可用。
3. IVR 素材缺失：`command.failed` + `call.ended` 带 `IVR_PROMPT_FAILED`。
4. 录音目录不可写：`recording.failed` toast。
5. Direct `leg.failed`：管理 runtime 可见 `end_message` 或 leg 失败提示。


### 应用播放结果

`leg.playback_started/finished/stopped/failed` 携带 `call_id`、`leg_id`、`playback_id`。完成事件的 `completion_scope=server_output` 只表示服务器输出完成及尾帧余量结束，不代表对端听见。call 的通知任务按播放结果决定完成或失败并请求挂断，Switch 不解释通知业务。

PCM WebSocket 的 `output.finished` 是连接内媒体事件，不属于业务通知结果。机器人按模型回复及 generation 推进对话，普通坐席仍使用原通话事件与终端媒体协议。
