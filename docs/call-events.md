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

## HTTP CallView 扩展

`GET /switch/v1/internal/calls/{id}` 与 BFF `getCall` 增加：

- `result` — 与 CDR/ended 一致（ended 后可从话单推断）
- `end_message` — 与 WS `message` 对齐
- `error_code`
- `pstn_dial_state` — 外呼 PSTN：`pending` / `dialing` / `connected` / `failed`（进行中通话）

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
