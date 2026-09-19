# 实时事件契约

> 对齐：[requirements.md](./requirements.md) EVT-*、[technical-design.md](./technical-design.md)  
> REST 管理面见 [docs/api/](./api/)

业务 **WebSocket 不承载 SDP**；WebRTC 信令走 HTTPS（OpenAPI Media Signaling）。

---

## 1. 连接

- **URL**：`wss://{host}/api/v1/ws?token={access_token}`  
- **token**：坐席/管理员 JWT access token，或访客 `guest_session` token（ADM-05）  
- **子协议**：无（JSON 文本帧）  
- **心跳**：客户端每 30s 发送 `{"type":"ping"}`；服务端回复 `{"type":"pong"}`  

---

## 2. 信封格式

```json
{
  "type": "call.ringing",
  "ts": "2026-09-18T10:00:00Z",
  "payload": { }
}
```

- `type`：事件类型（见下文）  
- `ts`：RFC3339 UTC  
- `payload`：类型相关对象  

---

## 3. 服务端 → 客户端

| type | 说明 | 主要 payload 字段 |
|------|------|-------------------|
| `call.ringing` | 振铃 | `call_id`, `queue_id`, `queue_name`, `caller`, `session_type` |
| `call.answered` | 接通 | `call_id`, `agent_id`, `answered_at` |
| `call.ended` | 结束 | `call_id`, `reason`, `result` |
| `call.hold` | 保持 | `call_id`, `leg_id` |
| `call.unhold` | 恢复 | `call_id`, `leg_id` |
| `call.transferring` | 转接中 | `call_id`, `target_agent_id`, `mode` |
| `call.consulting` | 咨询转目标已接听 | `call_id`, `agent_id` |
| `call.transferred` | 咨询转完成 | `call_id`, `from_agent_id` |
| `call.voicemail` | 留言箱放音 | `call_id`, `message` |
| `agent.state_changed` | 坐席状态 | `agent_id`, `state`, `busy_reason` |
| `queue.stats` | 队列统计 | `queue_id`, `waiting`, `longest_wait_sec` |
| `queue.position` | 排队位置 | `call_id`, `position`, `message` |
| `video.requested` | 升视频请求 | `call_id`, `from_leg_id` |
| `video.accepted` | 同意视频 | `call_id` |
| `video.declined` | 拒绝视频 | `call_id` |
| `video.downgraded` | 降级语音 | `call_id`, `session_type` |
| `screen_share.started` | 屏幕共享开始 | `call_id`, `leg_id` |
| `screen_share.stopped` | 屏幕共享结束 | `call_id`, `leg_id` |
| `queue.overflow` | 溢出到另一队列 | `call_id`, `queue_id` |
| `ivr.started` | IVR 开始 | `call_id`, `snapshot_id` |
| `ivr.prompt` | IVR 提示 | `call_id`, `prompt`, `type` |
| `call.supervisor_listen` | 班长加入监听 | `call_id`, `leg_id`, `announced` |
| `recording.notice` | 录制告知 | `call_id`, `message` |
| `pong` | 心跳响应 | — |

### 3.1 示例：`call.ringing`

```json
{
  "type": "call.ringing",
  "ts": "2026-09-18T10:00:01Z",
  "payload": {
    "call_id": "550e8400-e29b-41d4-a716-446655440000",
    "queue_id": "q-sales",
    "queue_name": "销售队列",
    "caller": "guest:abc123",
    "session_type": "video"
  }
}
```

### 3.2 示例：`agent.state_changed`

```json
{
  "type": "agent.state_changed",
  "ts": "2026-09-18T10:05:00Z",
  "payload": {
    "agent_id": "agent-001",
    "state": "busy",
    "busy_reason": "break"
  }
}
```

### 3.3 示例：`video.requested`

```json
{
  "type": "video.requested",
  "ts": "2026-09-18T10:10:00Z",
  "payload": {
    "call_id": "550e8400-e29b-41d4-a716-446655440000",
    "from_leg_id": "leg-agent-1"
  }
}
```

---

## 4. 客户端 → 服务端

| type | 说明 | payload |
|------|------|---------|
| `ping` | 心跳 | — |
| `call.answer` | 接听 | `call_id` |
| `call.decline` | 拒接 | `call_id`, `reason` |
| `agent.set_state` | 示闲/示忙等 | `state`, `busy_reason` |
| `video.respond` | 升视频响应 | `call_id`, `accept`: boolean |
| `call.wrap_up` | 通话小结 | `call_id`, `text` |

### 4.1 示例：`call.answer`

```json
{
  "type": "call.answer",
  "payload": {
    "call_id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

---

## 5. FSM 触发对照

| 事件 | Call FSM / Agent FSM |
|------|----------------------|
| `call.ringing` | Call: queued → ringing |
| `call.answer` → 成功 | Call: ringing → active；Agent: ringing → on_call |
| `call.ended` | Call → ended |
| `agent.set_state` | Agent 状态迁移 + `agent.state_changed` 广播 |
| `video.*` | Call session_type / 重协商（L3） |

---

## 6. Webhook（EVT-04）

与 WS **同一进程内事件总线**；订阅配置见 OpenAPI `Webhooks`。

- **HTTP POST** 至订阅 URL  
- **Header**：`X-Open-VoIP-Signature: sha256=...`（HMAC-SHA256(body, secret)）  
- **Body**：与 WS 信封相同（`type`, `ts`, `payload`）  

**重试（折中）**：同步重试最多 N 次（配置）；仍失败写入 `webhook_deliveries.status=failed`；管理员调用 `POST /api/v1/webhooks/deliveries/{id}/retry`（见 [technical-design §2.10](./technical-design.md)）。

---

## 7. 错误与重连

| 场景 | 行为 |
|------|------|
| 401 token 无效 | 关闭 WS；客户端跳转登录/重新获取 guest token |
| guest token 过期 | 关闭 WS + `type=error` 帧（可选） |
| 网络断开 | 客户端指数退避重连；重连后携带新 token |

---

## 修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| v0.1 | 2026-09-18 | 初稿 |
