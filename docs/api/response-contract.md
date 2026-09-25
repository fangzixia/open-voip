# 统一 API 响应（1.1）

适用范围：浏览器 `/api/v1`、CC 提供的 `/platform/v1`、Switch 提供的 `/switch/v1`。请求体继续使用各接口的业务字段，无需增加外层包装。

成功示例（HTTP 200/201/202）：

```json
{"code":"OK","message":"成功","data":{"items":[]},"request_id":"request-123"}
```

失败示例（HTTP 409）：

```json
{"code":"AGENT_BUSY","message":"坐席忙","data":null,"request_id":"request-123","error":"conflict"}
```

- `code` 为稳定字符串；成功固定 `OK`，失败优先使用业务码，没有业务码时采用大写错误分类，如 `UNAUTHORIZED`、`RATE_LIMITED`、`SWITCH_UNAVAILABLE`。
- `message` 用于显示；`data` 承载原业务响应，包括列表的 `items`/分页字段。无返回数据的操作改为 HTTP 200 + `data:null`，不再返回 204。
- `request_id` 与响应头 `X-Request-ID` 一致；同步 CC → Switch、Switch → CC 调用透传请求 ID。异步回调产生新的请求 ID。
- `trace_id` 通过 `X-Trace-ID` 贯穿浏览器、CC、Switch 及异步 Platform 回调；缺失时由入口服务生成并在响应头返回。
- `X-Call-ID`、`X-Leg-ID` 和 `X-Client-Session-ID` 用于补充通话、媒体腿和浏览器会话关联，不替代业务路径中的 ID 校验。
- 保留正确的 HTTP 状态码。`error` 是失败时的兼容分类字段；不要通过匹配 `message` 判断业务逻辑。
- 鉴权、限流、404、405、处理器异常、BFF 连接失败都采用同一 JSON 格式。

## 日期与时间

- 含具体时刻的 JSON 属性统一使用 UTC `YYYY-MM-DD HH:MM:SS`，例如 `2026-09-25 10:12:35`。
- 仅表示日历日期的属性和 `format: date` 查询参数使用 `YYYY-MM-DD`，例如 `2026-09-25`；历史报表的 `from`、`to` 是包含首尾两天的日期范围。
- Go 业务与数据库字段继续使用 `time.Time`。服务边界通过两个模块的 `internal/datetime` 编解码；纯日期字段使用 `datetime.Date` 或 `datetime:"date"` 标记。新增 JSON 边界应调用该包，避免直接对含时间字段的结构使用 `encoding/json`。
- 新请求按上述格式校验；服务间读取旧消息时暂兼容 RFC3339，输出始终使用统一格式。不再使用 `tools/generate_datetime.go` 或 `zz_datetime_json.go` 生成文件。

## 协议例外

录音/CSV 成功返回原始文件流；失败返回统一 JSON。`/health`、`/health/live` 保留纯文本存活探针。CORS OPTIONS 保留 204。WebSocket 握手前的应用鉴权失败返回 JSON，升级后的事件仍采用既有事件协议。

## 前端

`shared/http-client.js` 是唯一 HTTP 传输入口，JSON 与下载共用鉴权、错误处理和超时逻辑。业务 API 返回已解包的 `data`，页面无需读取外层字段。

通话页面通过共享观测模块批量上报 `/api/v1/client-events`。上报内容不得包含令牌、SDP、完整 ICE candidate、TURN 凭据和设备标签；服务器再次脱敏后只写 `logs/open-call/trace.jsonl`。

## 服务器追踪日志

- `open-call` 与 `open-switch` 分别输出 `app.jsonl` 和 `trace.jsonl`。
- 文件达到 `log.max_size_mb` 后按时间戳轮转并 gzip 压缩；配置的保留天数为 `0` 时永久保留。
- SIP/SDP/ICE 正文保留调试结构，但 Authorization、Cookie、Digest response、密码、TURN credential 与 SDP `ice-pwd` 始终替换为 `[REDACTED]`。
- 主要关联字段为 `trace_id`、`request_id`、`call_id`、`leg_id`、`agent_id` 和 `queue_id`。通话结束会写入 `call.trace.summary`。

默认超时 30 秒，文件下载 120 秒；支持 `signal` 取消。网络失败、超时、取消分别使用 `NETWORK_ERROR`、`TIMEOUT`、`CANCELED`。不自动重试有副作用的操作。`ApiError` 保留 `status`、`code`、`body`、`requestId`。

三端通过 `bindApiFeedback` 统一显示错误。受保护请求返回 401 时清理当前令牌，通知页面恢复登录/入会状态并关闭相关连接；匿名登录失败不清理其他会话，旧请求不能清除新登录的令牌。本版采用重新登录策略，不自动使用 refresh token。退出登录调用后端撤销接口后清理本地会话。

## 升级

前端与两个服务同时发布。内部客户端及前端暂兼容旧版裸 JSON/空响应，便于滚动升级；服务端只输出新格式。外部直连调用方须改为读取 `data`，并接受原 204 改为 200。BFF 透传 Switch 的统一结构，不重复包装。`openapi.yaml` 已更新浏览器接口的响应 schema；CC/Switch 文档中的业务 DTO 均位于 `data` 内。

## 验证

在两个 Go 模块分别执行 `go test ./...`；前端执行 `npm test`、`npm run build`。契约测试覆盖响应字段、错误状态、异常恢复、双向客户端解包与请求 ID、401 会话隔离、下载、网络失败和超时。
