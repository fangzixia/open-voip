# open-switch 对接说明

适用于 `/switch/v1`。open-switch 是部署在内网的软交换后端；外部应用负责用户认证、权限、业务路由、坐席、队列、话单和录音业务数据。Switch 只验证调用方的服务密钥，信任服务命令中的 `agent_id`、`leg_id` 等字段，不解释终端用户身份。

## 部署模式与信任边界

`integration.mode` 可设为：

| 模式 | 用途 | 业务依赖 |
| --- | --- | --- |
| `call_center`（默认） | 与 open-call 配套，沿用队列、ACD、坐席、IVR 流程 | 需要 `integration.platform_base_url` |
| `external` | 其他应用直接编排呼叫和媒体腿 | 不调用 Platform API |

两种模式都要求 `integration.secret`（至少 8 个字符）。每个 `/switch/v1` 请求携带 `Authorization: Bearer <integration.secret>`；`/health` 无需密钥。Switch API 只在可信内网暴露，前端与终端设备通过自己的应用后端访问。`X-Principal` 不再参与 Switch 鉴权，Switch 不校验用户、坐席、号码、队列的业务权限。调用应用必须在发命令前完成这些检查，隔离不同租户，并限制可拨号码和 SIP 中继。

### 模式配置

```yaml
integration:
  mode: external
  secret: "replace_with_a_shared_service_secret"
  platform_base_url: "" # external 模式可留空
```

既有 open-call 部署保持 `mode: call_center`，并继续配置 `platform_base_url`。切换模式前应结束现有通话；媒体会话无法跨进程重启恢复，Switch 启动时会将未结束的通话标记为异常结束。

## external 模式：控制一通双腿呼叫

请求和响应使用 JSON。成功响应统一为 `{"code":"OK","message":"成功","data":...}`，下文的 CallView 和列表均位于 `data`。`call_id` 如由应用提供，必须是 UUID；重复提交相同的 `call_id` 和呼叫字段会返回原通话，字段不一致返回 `409`。不提供时 Switch 生成 UUID。当前每通 external 呼叫最多两个媒体腿，只支持一对一桥接；会议、盲转、咨询转和 ACD 属于 `call_center` 模式的接口。

1. `POST /switch/v1/calls/direct` 创建通话。请求示例：

   ```json
   {"call_id":"c636f668-2af5-4c45-9853-0c5de1024eb1","direction":"outbound","caller":"1001","callee":"13800000000","session_type":"audio","initial_leg_role":"agent","agent_id":"a-01"}
   ```

   `direction` 为 `inbound|outbound|internal`，默认 `inbound`；`session_type` 为 `audio|video`，默认 `audio`；`initial_leg_role` 为 `customer|agent|pstn`，默认 `customer`；`agent_id` 可关联第一腿的坐席。响应 `201` 为 CallView，含 `id`、`state: created`、`legs[].id`。若第一腿是 WebRTC，可在创建后直接完成 Offer/Answer/ICE。

2. 添加第二腿：

   - `POST /switch/v1/calls/{callId}/legs`，body `{"role":"agent","agent_id":"a-01"}`，创建 WebRTC 腿，返回 `201` CallView。`role` 允许 `customer|agent|supervisor`。
   - 或 `POST /switch/v1/calls/{callId}/legs/sip`，body `{"destination":"13800000000","trunk_id":"trunk-1"}`，创建 PSTN 腿并异步拨号，返回 `202` CallView。通过事件 `leg.dialing`、`leg.answered`、`leg.failed` 判断拨号结果。

3. WebRTC 信令：`POST /calls/{callId}/legs/{legId}/offer` 获取本地 SDP；`POST .../answer` 提交 `{"sdp":"...","type":"answer"}`；`POST .../ice` 提交 ICE candidate；`POST .../mute` 控制本腿静音。上述路径均带 `/switch/v1` 前缀。`GET /calls/{callId}/turn-credentials?subject=<应用侧主体>` 获取临时 TURN 凭据。

4. 双腿媒体均已连接后，调用 `POST /switch/v1/calls/{callId}/bridge`，body `{"leg_a":"...","leg_b":"..."}`。桥接前不会互相转发媒体；桥接成功后通话进入 `active`，产生 `call.answered`。删除其中一腿会撤销桥接。

5. `DELETE /switch/v1/calls/{callId}/legs/{legId}` 移除单腿；`POST /switch/v1/calls/{callId}/hangup` 结束整通呼叫。`GET /switch/v1/calls/{callId}` 读取通话状态。`GET /switch/v1/internal/calls/{callId}` 是应用后端使用的同类视图。

SIP 入呼和设备 INVITE 也会在 external 模式自动创建以 PSTN 为第一腿的 direct 呼叫，发出 `call.created`。应用从事件中取得 `call_id` 和 `leg_id`，添加第二腿后桥接。SIP 监听地址、路由和中继仍由 `sip` 配置决定。

### 通用媒体控制

两种模式都提供 `hangup`、`video/request`（body `from_leg_id`）、`video/respond`、`video/downgrade`、`screen-share`（body `leg_id`、`on`）、`dtmf`（body `leg_id`、`digit`）和 WebRTC 信令。录制只在 external 模式由控制器显式发起：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/switch/v1/calls/{callId}/recording/start` | body `{"mode":"audio"}` 或 `{"mode":"video_composite"}`，返回录制元数据 |
| POST | `/switch/v1/calls/{callId}/recording/stop` | 返回文件路径、大小等元数据 |
| GET | `/switch/v1/internal/recordings/{callId}/{recordingId}` | 获取录制文件 |
| DELETE | 同上 | 删除录制文件 |

应用自行决定录制策略、告知、权限、留存、话单关联和文件访问控制。Switch 保存文件并返回元数据，不在 external 模式写应用的话单或调用录制元数据回调。

## 事件与重放

`GET /switch/v1/events?after_id=0&limit=100` 的 `data` 为 `{"items":[...]}`，可用 `call_id` 过滤单通通话。每项包含全局递增 `id`、通话内 `seq`、`call_id`、`type`、`agent_id`、`target_only`、`payload`、`created_at`。按 `id` 升序读取，处理成功后保存最后一个 `id`，下次作为 `after_id`；单次最多返回 500 项。`target_only: true` 表示事件原本只发给指定坐席，应用应按 `agent_id` 定向分发，避免对所有通话参与者重复广播。

事件持久化后可按游标重放；消费端应按事件 `id` 去重，并在启动或断线后用 `GET /switch/v1/internal/calls`、`GET /switch/v1/calls/{callId}` 对账。事件写入与通话状态更新目前不是同一个数据库事务，个别内部事件发布失败也可能只留下通话状态；因此事件不能作为唯一的最终状态依据。事件表目前不自动清理，部署时需监控增长，确定所有消费方的保留窗口后再设计清理策略。

## call_center 模式与 open-call

原有 Platform API、队列、ACD、坐席、IVR 和话单流程仍在 `call_center` 模式运行。Switch API 的 `answer`、`decline`、`outbound`、班长监听等命令现在用请求体中的 `agent_id`，升视频用 `from_leg_id`，TURN 用 `subject` 查询参数；Switch 不再读取 `X-Principal`。open-call BFF 验证终端令牌、权限、通话及媒体腿归属后，填入受信任的字段并转发；其服务端 Switch 客户端也使用显式字段。open-call 从 Switch 事件流按数据库游标轮询并分发 WebSocket / webhook，payload 会增加 `switch_event_id` 与 `switch_call_seq`；故障重试可能重复分发，接收方可按 `switch_event_id` 去重。

`call_center` 独有路径包括 `/calls/inbound`、`/calls/outbound`、`/calls/{id}/answer|decline|hold|transfer|transfer/complete|conference` 和 `/supervisor/...`；external 模式不注册这些路径。`/switch/v1/ivr-assets` 在两种模式下可用于管理音频资源。

## 部署与联调检查

1. 为 Switch 使用独立 PostgreSQL 数据库；启动时执行迁移，创建 `os_call_events`。open-call 的数据库迁移会创建 `oc_switch_event_cursor`。
2. 限制 Switch HTTP、SIP、RTP 的网络访问范围；服务密钥仅交给可信控制器。应用后端负责终端认证和业务授权。
3. 用固定 `call_id` 创建测试呼叫，验证重复提交、第二腿、信令、桥接、挂断和事件游标重放；SIP 通话另测中继失败与 `leg.failed`。
4. 在 open-call 配套场景，先部署支持显式字段和事件轮询的 open-call，再部署本版 Switch；检查 BFF 转发与事件消费。两端均完成迁移后再开放业务流量。

Switch API 仍为 `/switch/v1`，但信任边界和部分字段已变化，旧版控制器需要同步升级。返回 `4xx` 表示请求或状态错误，`5xx` 表示 Switch 或依赖故障；客户端对重试命令应使用稳定 `call_id`，并在超时后先查询通话状态。
