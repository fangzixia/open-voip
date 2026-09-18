# Open VoIP REST API 对接指南

> 机器可读契约：[openapi.yaml](./openapi.yaml)  
> 实时事件：[../events.md](../events.md)  
> 架构与分层：[../architecture.md](../architecture.md)

本文面向 **第三方客户端**（自建坐席 UI、CRM 集成、运维脚本），无需阅读 Go 源码。

---

## 1. 概述

- **风格**：REST JSON，版本前缀 `/api/v1`  
- **Base URL 示例**：`https://cc.internal`（以内网 `config.yml` 中 `public_url` 为准）  
- **字符编码**：UTF-8  
- **时间**：RFC3339 UTC，除非字段说明为本地业务时区  

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

访客不使用用户名密码。先调用 `POST /api/v1/guest/sessions`（管理员或系统签发）获取 **guest token**，再：

- REST：部分只读/入会接口  
- WebSocket：`wss://.../api/v1/ws?token={guest_token}`  

---

## 3. 与 WebSocket 的分工

| 能力 | 通道 |
|------|------|
| CRUD、CDR 查询、配置 | **REST** |
| 振铃、状态变更、排队位置 | **WebSocket**（[events.md](../events.md)） |
| SDP / ICE / Offer-Answer | **REST** Media Signaling（见 OpenAPI tag `MediaSignaling`） |

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

CDR 等支持 `from`, `to`（ISO8601）、`queue_id`、`agent_id` 过滤（见 OpenAPI）。

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
- `GET /api/v1/status` — `{ "db_ok": true, "active_calls": 0, "ws_connections": 0 }`  

**说明**：当前版本不提供 Prometheus `/metrics` 与主机 CPU/磁盘采集（见 [technical-design §2.10](../technical-design.md)）。

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
