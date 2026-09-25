# Open VoIP REST API 对接指南

> 机器可读契约：[openapi.yaml](./openapi.yaml)  
> 实时事件：[../events.md](../events.md)  
> 架构与分层：[../architecture.md](../architecture.md)

本文面向 **第三方客户端**（自建坐席 UI、CRM 集成、运维脚本），无需阅读 Go 源码。

---

## 1. 概述

- **风格**：REST JSON，版本前缀 `/api/v1`  
- **Base URL 示例**：`https://cc.internal`  
- **字符编码**：UTF-8  
- **时间戳**：统一为 UTC 的 `YYYY-MM-DD HH:MM:SS`，例如 `2026-09-25 10:12:35`；请求、响应、WebSocket、Webhook 和 CSV 均使用此格式。数据库和内部计算保留时间类型。无时区后缀，调用方按 UTC 解释。仅日期型业务字段仍为 `YYYY-MM-DD`。

服务端在 JSON 边界使用 `internal/datetime` 统一处理 `time.Time`；纯日期字段使用 `datetime.Date` 或 `datetime:"date"` 标记，不需要生成代码。普通字符串不会被识别或改写为时间。

---

## 2. 认证（PLAT-02）

### 2.1 登录

```http
POST /api/v1/auth/login
Content-Type: application/json

{"username":"agent1","password":"***"}
```

响应：

```json
{
  "access_token": "eyJ...",
  "refresh_token": "eyJ...",
  "expires_in": 3600,
  "token_type": "Bearer"
}
```

### 2.2 调用受保护 API

```http
GET /api/v1/agents/me
Authorization: Bearer {access_token}
```

### 2.3 刷新与登出

- `POST /api/v1/auth/refresh` — body: `{ "refresh_token": "..." }`  
- `POST /api/v1/auth/logout` — 撤销 refresh / jti  

### 2.4 访客

访客不使用用户名密码。Phase 1 Demo：

1. `GET /api/v1/guest/queues`（无需登录）列出队列  
2. `POST /api/v1/guest/join` `{ "queue_id", "session_type": "audio"|"video" }` 得到 `token`、`call_id`、`leg_id`  
3. WebSocket：`ws(s)://.../api/v1/ws?token={token}`  
4. 坐席接听后对 `leg_id` 走 Media Signaling REST  

管理员也可调用 `POST /api/v1/guest/sessions` 签发 `token` 与相对路径 `guest_url`（如 `/guest/?token=`）；绝对链接由调用方用自己的 UI 根地址拼接。  

---

## 3. 与 WebSocket 的分工

| 能力 | 通道 |
|------|------|
| CRUD、CDR 查询、配置 | **REST** |
| 振铃、状态变更、排队位置 | **WebSocket**（[events.md](../events.md)） |
| SDP / ICE / Offer-Answer | **REST** Media Signaling（见 OpenAPI tag `MediaSignaling`） |

### 3.1 全链路追踪

- 客户端发送 `X-Trace-ID`；服务端在缺失时生成并通过响应头返回。
- 使用 `X-Request-ID` 定位单次 HTTP 请求，使用 `call_id`/`leg_id` 定位整通电话和媒体腿。
- 浏览器通话诊断通过 `POST /api/v1/client-events` 批量上报。
- 服务器日志位于配置的 `log.dir`，其中 `trace.jsonl` 可直接按 `trace_id` 或 `call_id` 检索。轮转文件 gzip 压缩且默认永久保留。

---

## 4. 分页与过滤

列表接口统一 query：

- `page`（从 1 开始，默认 1）  
- `page_size`（默认 20，最大 100）  

响应 envelope：

```json
{
  "items": [],
  "page": 1,
  "page_size": 20,
  "total": 0
}
```

CDR 等支持 `from`, `to`（`YYYY-MM-DD HH:MM:SS`，UTC）、`queue_id`、`agent_id` 过滤（见 OpenAPI）。

---

## 5. 错误响应

标准 HTTP 状态码 + JSON body，详见 [errors.md](./errors.md)。

```json
{
  "error": "invalid_request",
  "message": "队列不存在",
  "details": {}
}
```

业务错误可含 `code`（如 `AGENT_NOT_VIDEO_CAPABLE`）。

---

## 6. 角色（PLAT-03）

| 角色 | 说明 |
|------|------|
| admin | 用户/队列/IVR/CDR 管理 |
| supervisor | 班长：监听、强制签出、录音下载 |
| agent | 签入、通话、小结 |

OpenAPI 各 operation 标注 `x-roles` 供代码生成与评审。

---

## 7. 健康与状态（PLAT-06 / DEPLOY-08）

- `GET /health` — 进程存活，200 即 OK  
- `GET /api/v1/status` — `{ db_ok, active_calls, ws_connections, goroutines, heap_alloc_bytes, num_cpu }`（MON-01）

Phase 2/3 补充：IVR、录音、转接/外呼、报表、Webhook、质检、DID、班长监听等路径见 OpenAPI tags `IVR` / `Recordings` / `Reports` / `Webhooks` / `Supervisor`。

---

## 8. 工具链

- **校验 OpenAPI**：`npx @redocly/cli lint openapi.yaml --config redocly.yaml`  
- **浏览文档**：`npx @redocly/cli build-docs openapi.yaml -o redoc.html`  
- **导入**：Postman / Apifox 直接导入 `openapi.yaml`  

---

## 9. 版本策略

- URL 路径含 `/v1`；破坏性变更递增 major 路径  
- 字段只增不减；废弃字段在 OpenAPI `deprecated: true` 保留至少一版  

---

## 修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| v0.1 | 2026-09-18 | 初稿 |
