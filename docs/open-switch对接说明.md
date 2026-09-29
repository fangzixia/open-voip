# open-switch 对接说明

适用于 **`/switch/v2`**。open-switch 是部署在内网的软交换后端：承载 SIP/WebRTC 媒体、通话状态机，以及在 Switch 库内激活的 DID、队列、ACD、坐席话务状态与 IVR 执行。业务系统（范例为 open-call，也可为第三方）负责终端用户认证与权限、业务主数据编辑、配置发布、事件消费，以及需要查业务库的 IVR `business_action` 决策。

Switch **不会**在呼叫路径上同步查询业务库，也**没有** `integration.mode`、`platform_base_url` 或 `/platform/v1`。运行时队列/ACD/IVR 只读 Switch 内已激活的配置快照。Switch 在事件落库后向业务系统登记的 **`events_callback_url` POST 推送**（非终端用户回调）。

## 鉴权与信任边界

业务系统调用 `POST /switch/v2/integrations/register`（`X-Register-Token`）登记 `application_id` 与 `events_callback_url`；Switch **生成** integrator `secret` 并在新建时返回一次。除 `/health` 与登记接口外，每个 `/switch/v2` 请求须携带：

```http
Authorization: Bearer <Switch 签发的 secret>
```

校验通过后绑定 `application_id` 租户作用域。open-call 在 YAML 中配置 `integration.switch_base_url`、`application_id`、`secret`（register 获得）、`events_callback_url`。

Switch 信任请求体中的 `agent_id`、`leg_id`、`queue_id` 等字段，**不**校验终端用户身份。`X-Principal` 不参与 Switch 鉴权。业务系统必须在 BFF 完成 JWT/权限检查。

### 配置示例（Switch）

```yaml
integration:
  register_token: "replace_with_random_register_token_32+"
  allow_open_register: false
```

### 配置示例（open-call）

```yaml
integration:
  switch_base_url: "http://127.0.0.1:8082"
  application_id: "cc-prod"
  secret: "<register 响应 data.secret>"
  events_callback_url: "http://127.0.0.1:8080/api/v1/integration/switch/events"
  register_token: "..."
```

登记步骤见 [switch-standalone-runbook.md](./switch-standalone-runbook.md)。

Switch API 仅在内网暴露；浏览器与 SIP 终端通过业务系统 BFF 或自建后端访问。

## 集成方式（非互斥）

同一 Switch 进程同时注册「双腿编排」与「呼叫中心」接口，不再用 `call_center` / `external` 模式开关裁剪路由。

| 方式 | 典型调用方 | 说明 |
| --- | --- | --- |
| **open-call 配套** | open-call BFF + `switchapi` | 发布配置到 Switch；BFF 鉴权后调用 Switch；**HTTP callback** 接收事件并投影/WebSocket |
| **自建控制器** | 任意持 secret 的后端 | `calls/direct`、呼叫中心 API；登记 callback URL；可选 `GET /events` 对账 |

升级或切换集成方式前宜结束现有通话。媒体房间无法跨进程保留：Switch 启动时会尝试从 `os_ivr_sessions` 恢复 **IVR**，从库表恢复 **排队/振铃** 并重新建媒体与派单；**通话中/保持/转接中** 等已桥接状态仍会收尾结束，客户端需重新建链。

## 双腿编排（`calls/direct`）

请求与响应为 JSON。成功响应为 `{"code":"OK","message":"成功","data":...}`，下文 CallView 位于 `data`。`call_id` 若由应用提供须为 UUID；相同 `call_id` 与字段重复提交返回原通话，字段不一致返回 `409`。未提供时 Switch 生成 UUID。每通 direct 呼叫当前最多两个媒体腿，默认一对一桥接；会议、咨询转、盲转等使用下文呼叫中心接口。

1. `POST /switch/v2/calls/direct` 创建通话。示例：

   ```json
   {"call_id":"c636f668-2af5-4c45-9853-0c5de1024eb1","direction":"outbound","caller":"1001","callee":"13800000000","session_type":"audio","initial_leg_role":"agent","agent_id":"a-01"}
   ```

   `direction`：`inbound|outbound|internal`；`session_type`：`audio|video`；`initial_leg_role`：`customer|agent|pstn`；`agent_id` 可关联第一腿坐席。

2. 添加第二腿：

   - `POST /switch/v2/calls/{callId}/legs`，body `{"role":"agent","agent_id":"a-01"}`，WebRTC 腿，`role` 允许 `customer|agent|supervisor`。
   - 或 `POST /switch/v2/calls/{callId}/legs/sip`，body `{"destination":"13800000000","trunk_id":"trunk-1"}`（可用 `route_group_id` 作中继别名），建议带 `Idempotency-Key`；响应含 `command_id`、`leg_id`；`GET /switch/v2/commands/{commandId}` 查询拨号结果；事件 `leg.connected` / `command.failed` 等判断结果。

3. WebRTC 信令（均带 `/switch/v2` 前缀）：`.../legs/{legId}/offer|answer|ice|mute`；`GET .../turn-credentials?subject=<应用侧主体>` 获取 TURN 凭据。

4. 双腿媒体就绪后 `POST /switch/v2/calls/{callId}/bridge`（别名 `/bridges`，body 可用 `leg_ids`），body `{"leg_a":"...","leg_b":"..."}`。桥接成功后产生 `bridge.active` 与 `call.answered`（事件 payload 可含 `bridge_id`）；Switch 同步写入运行库表 `os_bridges`。IVR 进行时在 `os_ivr_sessions` 记录当前节点与超时，入队或挂断时清除。

5. `DELETE /switch/v2/calls/{callId}/legs/{legId}` 移除单腿；`POST .../hangup` 结束呼叫；`GET .../calls/{callId}` 或 `GET .../internal/calls/{callId}` 查询状态。

SIP 入呼在 Switch 侧按**已激活 DID 快照**路由到队列或 IVR，无需业务系统同步查询。设备外呼等场景亦可从事件取得 `call_id` 后再编排第二腿。

### 显式录制（编排场景）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/switch/v2/calls/{callId}/recording/start` | `{"mode":"audio"}` 或 `{"mode":"video_composite"}` |
| POST | `/switch/v2/calls/{callId}/recording/stop` | 返回文件元数据 |
| GET/DELETE | `/switch/v2/internal/recordings/{callId}/{recordingId}` | 读写录音文件 |

队列入呼的录制策略由配置快照中的 `recording_policy` 等字段驱动，由 Switch 自动执行；业务系统通过 `recording.saved` 等事件同步元数据。

## 呼叫中心运行接口

在已发布配置与坐席签入前提下，业务系统（通常经 open-call BFF）调用：

- `POST /switch/v2/calls/inbound`、`POST /switch/v2/calls/outbound`
- `POST /switch/v2/calls/{callId}/answer|decline|hold|transfer|transfer/complete|conference`
- `POST /switch/v2/supervisor/calls/{callId}/listen`、`POST /switch/v2/supervisor/agents/{agentId}/force-check-out`

命令使用请求体中的 `agent_id`；升视频使用 `from_leg_id`；TURN 使用 `subject` 查询参数。

### 配置与坐席（管理面）

业务系统编译不可变配置包并写入 Switch（open-call 使用 `configpub`）：

- `POST /switch/v2/configuration/versions`（别名 `/config-versions`）— 存储快照
- `POST /switch/v2/configuration/versions/{version}/activate` — 激活
- `GET /switch/v2/configuration/versions/{version}` — 查看版本元数据
- `POST /switch/v2/calls` — 创建无媒体腿的空 Call（`business_ref` / `metadata`）
- `GET /switch/v2/routing-sessions/{callId}` — 路由会话与队列项只读快照

坐席运行时：

- `POST /switch/v2/agents/{agentId}/check-in|check-out`
- `PUT /switch/v2/agents/{agentId}/presence`
- `GET /switch/v2/agents/{agentId}/session`
- `GET /switch/v2/queues/{queueId}/status`

`GET/POST /switch/v2/ivr-assets` 管理 IVR 音频资源。

### IVR 业务判断节点

流程进入 `business_action` 节点时，Switch 发布 `business_action.requested` 并挂起，直到：

`POST /switch/v2/calls/{callId}/business-actions/{actionId}/complete`，body `{"outcome":"<已声明分支>"}`

业务系统须根据 `action` 类型查询自有业务库后回填 outcome；超时走 IVR 快照中的默认分支。Switch **不会**主动 HTTP 调用 open-call 完成该步骤。

### 通用媒体控制

`hangup`、`video/request`、`video/respond`、`video/downgrade`、`screen-share`、`dtmf` 与 WebRTC 信令在编排与呼叫中心场景共用。

## 事件与重放

写操作可在 JSON 中传 `expected_version`（与 `GET /calls/{callId}` 的 `version` 一致）；冲突返回 `409`、`VERSION_MISMATCH`，`data` 为最新通话视图。下列写操作支持 `Idempotency-Key`（记入 `os_commands`）：`hangup`、`answer`、`decline`、`hold`、`transfer`、`bridge` / `bridge.replace`、`leg.leave`、SIP 拨号等。`PUT /calls/{callId}/bridges/{bridgeId}` 可原子替换 `leg_ids`；`POST /calls/{callId}/legs/{legId}/hold`（body `on`）、`reject`、`playbacks` 与方案 §5 对齐。对账可用 `GET /switch/v2/calls?status=open`（返回 `items`）或 `GET /switch/v2/internal/calls`。

`GET /switch/v2/events?after_id=0&limit=100`，可用 `call_id` 过滤。`data` 为 `{"items":[...]}`，每项含全局递增 `id`、通话内 `seq`、`call_id`、`type`、`agent_id`、`target_only`、`payload`、`created_at`。按 `id` 升序消费，成功后以最后 `id` 作为下次 `after_id`；单次最多 500 条。`target_only: true` 时按 `agent_id` 定向分发。

open-call 在 callback 处理中写入 `oc_switch_event_cursor` 与出站队列，并发布 WebSocket；payload 附带 `switch_event_id`、`switch_call_seq`。Switch POST body 与 `/events` 单条 item 同 schema；响应须 `{"accepted":true,"event_id":N}`（包在标准 `data` 内亦可）。

事件与通话状态更新目前不是同一数据库事务；对账时使用 `GET /switch/v2/internal/calls` 与 `GET /switch/v2/calls/{callId}`。事件表需按 `event_retention_days` 与运维策略监控容量。

Switch 进程重启时：`ivr` / `queued` / `ringing` 会尝试恢复并重建媒体；**active** / **held** / **transferring** 会保留通话并发布 `call.media_reconnect_required`，客户端须重建 WebRTC/SIP 媒体。持久化 `UpdateCall` 与 `call.state_changed` 在同一事务；其余业务事件（如 `call.answered`）发布失败会使写操作返回错误。详见 [§9 验收清单](./section9-acceptance-checklist.md)。

## 部署与联调检查

1. Switch 使用独立 PostgreSQL（`os_*` 表）；open-call 使用业务库（`oc_*` 表，含 `oc_switch_event_cursor`）。禁止跨服务直连对方库表做运行时查询。
2. 限制 Switch HTTP、SIP、RTP 访问范围；应用密钥仅配置在服务端。
3. 发布配置 → 坐席签入 → 入呼/出呼 → 验证 `/events` 游标与 WebSocket。
4. 固定 `call_id` 测 direct 重复提交、桥接、挂断与事件重放；SIP 场景另测 `leg.failed` 等。

`/switch/v1` 与 Platform 同步查询已移除，旧版客户端须升级到 `/switch/v2`。`4xx` 为请求或状态错误，`5xx` 为 Switch 或依赖故障；重试命令应使用稳定 `call_id`，超时后先查询通话状态。
