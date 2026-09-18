# API 错误码

> 与 [openapi.yaml](./openapi.yaml) 中 `components/schemas/Error` 一致。

---

## HTTP 状态

| 状态 | 含义 |
|------|------|
| 400 | 参数校验失败 |
| 401 | 未认证或 token 失效 |
| 403 | 无权限（RBAC） |
| 404 | 资源不存在 |
| 409 | 状态冲突（如非 ringing 时 answer） |
| 422 | 业务规则拒绝 |
| 500 | 服务器内部错误 |

---

## error 字段（通用）

| error | 说明 |
|-------|------|
| `invalid_request` | 参数错误 |
| `unauthorized` | 认证失败 |
| `forbidden` | 权限不足 |
| `not_found` | 资源不存在 |
| `conflict` | 并发/状态冲突 |
| `internal_error` | 未预期错误 |

---

## code 字段（业务，可选）

| code | 场景 |
|------|------|
| `CALL_NOT_RINGING` | 非振铃态接听 |
| `AGENT_NOT_IDLE` | 分配时坐席不可用 |
| `AGENT_NOT_VIDEO_CAPABLE` | 视频转接目标不支持视频 |
| `QUEUE_FULL` | 队列等待已满 |
| `GUEST_TOKEN_EXPIRED` | 访客链接过期 |
| `SIP_DISABLED` | 未配置 PSTN trunk |
| `TRANSFER_FAILED` | 转接失败 |

---

## 修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| v0.1 | 2026-09-18 | 初稿 |
