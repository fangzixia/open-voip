# open-switch 业务对接协议

面向**第三方业务系统**开发人员：说明如何与 softswitch（open-switch）对接。  
业务系统扮演呼叫中心业务面角色；Switch 负责媒体、呼叫状态机、ACD/IVR/录音。

基路径：`{switch_base_url}/switch/v1`  
字段与路径以当前实现为准；冲突时以 Switch 代码为准。

---

## 1. 交互模型

```
业务系统  ──HTTP──►  open-switch /switch/v1     （命令、配置、信令）
open-switch ──HTTP POST──► 业务系统 callback URL （事件推送）
```

| 方向 | 通道 | 说明 |
|------|------|------|
| 业务 → Switch | REST JSON | 无 `Authorization`；依赖私有网络隔离 |
| Switch → 业务 | `POST events_callback_url` | 无 `Authorization`；主路径为推送，`GET /events` 仅用于对账 |

Switch 侧须配置完整回调地址，例如：

```yaml
integration:
  events_callback_url: "http://biz.internal/integration/switch/events"
```

业务侧只需知道 Switch 根地址（如 `http://127.0.0.1:8082`）。

### 1.1 请求头

| Header | 说明 |
|--------|------|
| `Content-Type` | `application/json` |
| `Idempotency-Key` | 变更类命令幂等键（建议必带） |
| `X-Request-ID` / `X-Trace-ID` | 可选追踪 |

### 1.2 统一响应信封

```json
{
  "code": "OK",
  "message": "成功",
  "data": { },
  "request_id": "..."
}
```

成功时业务载荷在 `data`。失败时 `code` / `error` / `message` 描述原因；常见 HTTP：`400` 校验、`403` 无激活配置、`404` 不存在、`409` 版本/状态冲突。

---

## 2. 通话控制

变更类请求可带 body `expected_version`（与当前 `CallView.version` 不一致则 `409`），以及 Header `Idempotency-Key`。

### 2.1 通用类型

**`session_type`**：`audio` | `video` | `mixed`

**`hangup.reason`**：`normal`（默认）| `timeout` | `abandon` | `error` | `transfer`

**`leg.role`**：`customer` | `agent` | `ivr_bot` | `supervisor` | `pstn`

**Call FSM `state`**：`created` | `ivr` | `queued` | `ringing` | `active` | `held` | `transferring` | `ended`

#### `CallView`（多数接口的 `data`）

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 通话 ID |
| `version` | int64 | 乐观锁版本 |
| `config_version` | int64? | 绑定的激活配置版本 |
| `state` | string | FSM 状态 |
| `direction` | string | inbound / outbound 等 |
| `session_type` | string | 媒介类型 |
| `queue_id` | string? | 当前队列 |
| `post_call_ivr_flow_id` | string? | 随通话队列快照固定的后续 IVR 流程标识；业务系统决定用途 |
| `agent_id` | string? | 振铃或接听坐席 |
| `caller` / `callee` | string | 主被叫标识 |
| `held` | bool? | 是否保持 |
| `recording_notice` | string? | 录音告知文案 |
| `terminal_type` | string? | 终端类型 |
| `created_at` / `answered_at` / `ended_at` | time | 时间戳 |
| `legs` | `LegView[]` | 媒体腿 |

#### `LegView`

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 腿 ID |
| `role` | string | 见 `leg.role` |
| `agent_id` | string? | 坐席腿时的坐席 ID |

### 2.2 创建与结束

| 方法 | 路径 | 请求体要点 | 响应 |
|------|------|------------|------|
| POST | `/calls/inbound` | `queue_id?`, `session_type?`, `priority?`, `caller?`, `skip_ivr?`, `call_id?`, `guest_session_id?` | `201` + `CallView` |
| POST | `/calls/outbound` | `agent_id`, `destination`, `trunk_id?` | `CallView` |
| POST | `/calls/{callId}/answer` | `agent_id`（须为当前 offered 坐席）, `expected_version?` | `CallView` |
| POST | `/calls/{callId}/decline` | `agent_id`, `expected_version?` | — |
| POST | `/calls/{callId}/hangup` | `reason?`, `expected_version?` | — |
| POST | `/calls/{callId}/hold` | `on` bool, `expected_version?` | — |

> inbound 的 `config_version` / `ivr_flow_id` 由 Switch（DID 等）注入，**不可**由客户端伪造。须先有激活配置，否则 `403`。

挂断顺序（Switch 侧）：释放坐席 → 停录音 → `cdr.updated` → `call.ended`。

坐席在振铃、通话或保持期间发起另一通外呼，返回 `409 AGENT_BUSY`，原通话与坐席状态保持不变。替换当前通话须由业务系统显式发送挂断，成功后再外呼；Switch 不自动结束旧通话。

### 2.3 转接 / 会议 / 主管

| 方法 | 路径 | Body |
|------|------|------|
| POST | `/calls/{callId}/transfer` | `mode`=`blind`\|`consult`；`target_agent_id` 与 `target_queue_id` 二选一；`expected_version?` |
| POST | `/calls/{callId}/transfer/complete` | 无（完成咨询转） |
| POST | `/calls/{callId}/conference` | `agent_id` |
| POST | `/supervisor/calls/{callId}/listen` | `agent_id` → `data.leg_id` |
| POST | `/supervisor/agents/{agentId}/force-check-out` | `policy` |

### 2.4 视频 / 屏幕共享 / DTMF

| 方法 | 路径 | Body |
|------|------|------|
| POST | `/calls/{callId}/video/request` | `from_leg_id` |
| POST | `/calls/{callId}/video/respond` | `accept` bool |
| POST | `/calls/{callId}/video/downgrade` | — |
| POST | `/calls/{callId}/screen-share` | `on`, `leg_id` |
| POST | `/calls/{callId}/dtmf` | `leg_id`, `digit` |

### 2.5 WebRTC 信令

| 方法 | 路径 | 请求 | 响应 `data` |
|------|------|------|-------------|
| POST | `/calls/{callId}/legs/{legId}/offer` | — | `{ sdp, type }` |
| POST | `/calls/{callId}/legs/{legId}/answer` | `sdp`, `type` | `{ ok }` |
| POST | `/calls/{callId}/legs/{legId}/ice` | `candidate`, `sdp_mid`, `sdp_mline_index?` | — |
| POST | `/calls/{callId}/legs/{legId}/mute` | `audio`, `video` | `{ ok }` |
| GET | `/calls/{callId}/turn-credentials?subject=` | — | TURN 凭证（含 `stun_urls`、`ur_ls`、`username`、`credential`、`ttl`） |

媒体权威在 Switch；业务系统只转发信令 API，不持有 RTP。

### 2.6 查询

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/calls/{callId}` | 单通话 `CallView` |
| GET | `/calls?status=open` | 未结束通话；`data.items[]` |
| GET | `/internal/calls` / `/internal/calls/{callId}` | 运行时查询 |
| GET | `/events?after_id=&limit=` | 事件对账（主路径仍是 callback） |
| GET | `/commands/{commandId}` | 幂等命令状态 |
| GET | `/routing-sessions/{callId}` | 路由/IVR 会话快照 |
| GET | `/internal/recordings/{callId}/{recordingId}` | 下载录音文件 |

### 2.7 Direct / 桥接 / 腿（高级编排）

| 能力 | 方法 | 路径 | 主要字段 |
|------|------|------|----------|
| 空 Call | POST | `/calls` | `call_id`, `business_ref?`, `metadata?` |
| 直控创建 | POST | `/calls/direct` | `call_id`, `direction`, `caller`, `callee`, `session_type`, `initial_leg_role`, `agent_id?` |
| 加腿 | POST | `/calls/{id}/legs` | `role`, `agent_id?` |
| SIP 腿 | POST | `/calls/{id}/legs/sip` | `destination`, `trunk_id?` |
| 离腿 | DELETE | `/calls/{id}/legs/{legId}` | — |
| 建/换/拆桥 | POST/PUT/DELETE | `/calls/{id}/bridges...` | `leg_a`/`leg_b` 或 `leg_ids[]` |
| 腿保持/拒接 | POST | `.../legs/{legId}/hold\|reject` | `on` / `reason` |
| 放音 | POST/DELETE | `.../playbacks` | `asset_id` |
| 开/停录 | POST | `.../recording/start\|stop` 或 `.../recordings...` | — |

### 2.8 IVR 业务动作回填

```http
POST /switch/v1/calls/{callId}/business-actions/{actionId}/complete
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `outcome` | string | 必须是流程声明的 `outcomes` 键之一 |

收到事件 `business_action.requested` 后须在 **deadline** 前调用本接口。相同 outcome 重放成功；不同 outcome → `409`；超时后 → `409`。

### 2.9 将客户接续到指定 IVR

`POST /switch/v1/calls/{callId}/ivr`：`{ "flow_id": "...", "expected_version": 12 }`，支持 `Idempotency-Key`，成功返回 `CallView`。`flow_id` 必填，通话须为 `active` 或 `held`，且具有客户或 PSTN 通话腿。Switch 先确认固定配置版本中的流程存在且可执行，再解除保持、停止原录音、释放服务方媒体腿及坐席占用，保留客户媒体并执行 IVR。Switch 不选择评价流程或解释评分。

open-call 保留浏览器业务入口 `POST /api/v1/calls/{callId}/survey`：鉴权并校验通话归属，使用明确传入的 `flow_id` 或 `CallView.post_call_ivr_flow_id`，然后调用上述通用接口。默认绑定来自该通话的配置快照，不读取后来修改的队列配置。

---

## 3. 配置与坐席运行态

**先激活配置，再产生呼叫。** 新激活仅影响新呼叫；在途通话绑定创建时的 `config_version`。

### 3.1 整包 Store + Activate

```http
POST /configuration/versions
POST /configuration/versions/{version}/activate
GET  /configuration/active
GET  /configuration/active/summary
```

`POST /configuration/versions` body：

| 字段 | 类型 | 说明 |
|------|------|------|
| `version` | int64? | 可选 |
| `queues` | QueueConfig[] | 队列 |
| `skills` | SkillConfig[] | 技能 |
| `agents` | AgentConfig[] | 坐席 |
| `dids` | DIDConfig[] | DID |
| `ivrs` | IVRConfig[] | 已发布 IVR |

响应 `ConfigVersion`：`version`, `status`, `checksum`, `created_at`, `activated_at?`。

适用：环境导入、整包备份恢复。日常单资源变更用 3.2。

### 3.2 资源级增量 API

每次写操作内部等价于「读当前激活包 → 改一处 → Store + Activate」。列表响应为 `{ "items": [...] }`（仍包在全局 `data` 内）；`DELETE` 多为 `204`。

#### 队列 `/queues`

| 方法 | 路径 | Body |
|------|------|------|
| GET | `/queues` | — |
| POST | `/queues` | `QueueConfig`（`id` 可空自动生成） |
| GET/PATCH/DELETE | `/queues/{queueId}/config` | PATCH 见合并语义 |
| PUT | `/queues/{queueId}/agents` | `{ agent_ids: string[] }` 整表替换 |
| PUT | `/queues/{queueId}/skills` | `{ skill_ids: string[] }` 整表替换 |

**QueueConfig 主要字段**

| 字段 | 说明 | 默认 |
|------|------|------|
| `id` / `name` | UUID / 名称（name 必填） | id 可生成 |
| `video_enabled` | 是否视频 | false |
| `max_wait_sec` | 最大等待秒 | 300 |
| `dispatch_strategy` | `longest_idle` \| `round_robin` | `longest_idle` |
| `recording_policy` | `off` \| `audio` \| `video_composite` | `off` |
| `overflow_action` | `hangup` \| `voicemail` \| `queue` | `hangup` |
| `overflow_queue_id` | 溢出目标队列（action=`queue` 时） | — |
| `ivr_flow_id` | 绑定已发布 IVR | — |
| `post_call_ivr_flow_id` | 后续交互流程标识；评价等业务用途由业务系统定义 | — |
| `audio_profile` | 本期仅接受 `narrowband`；`wideband` / `hd_webrtc` 发布或激活明确拒绝 | `narrowband` |
| `wait_prompt` / `announce_recording` / `priority_enabled` | 等候与录音告知、优先级 | — |
| `business_hours_json` / `after_hours_action` | 营业时间与非工作动作 | after=`hangup` |
| `force_hangup_on_checkout` / `listen_announce` | 签出强挂 / 监听告知 | false |
| `skill_ids` / `agent_ids` | 技能与坐席成员 | — |

技术能力枚举、等待时长、配置引用及流程图的最终校验统一由 Switch 执行。open-call 负责业务表单、默认值与部分更新合并，透传 Switch 的校验结果。`after_hours_action` 支持 `hangup` / `voicemail` / `queue`，`queue` 使用 `overflow_queue_id`。

PATCH 合并：一般字符串非空、数值非零才覆盖（负等待时长会交给 Switch 拒绝）；`audio_profile`、`ivr_flow_id`、`post_call_ivr_flow_id`、`overflow_queue_id` 区分省略与显式空字符串，省略保留、空字符串清除或恢复默认；`skill_ids`/`agent_ids` 非 null 才覆盖；若干 bool **始终覆盖**（漏传会变 `false`）。open-call 合并用户 PATCH 后发送完整配置及显式清空字段。改成员更推荐 `PUT .../agents|skills`。

#### 技能 `/skills`

| 方法 | 路径 | Body |
|------|------|------|
| GET/POST | `/skills` | POST: `{ id?, name }`（名称大小写不敏感唯一） |
| PATCH/DELETE | `/skills/{skillId}` | PATCH: `{ name }` |

#### 坐席配置 `/agents`

| 方法 | 路径 | Body |
|------|------|------|
| GET | `/agents/config` | — |
| PUT | `/agents/{agentId}/config` | `AgentConfig` Upsert |
| DELETE | `/agents/{agentId}/config` | — |
| PUT | `/agents/{agentId}/skills` | `{ skill_ids }` |

**AgentConfig**：`id`, `user_ref`（必填）, `extension`（必填、唯一）, `display_name?`, `video_capable`, `terminal_type`=`webrtc`\|`sip`, `sip_username?`（sip 必填）, `enabled`, `skill_ids?`。

#### DID `/did-routes`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | `/did-routes` | POST Upsert |
| PATCH/DELETE | `/did-routes/{didId}` | PATCH 仅非空字段覆盖 |

**DIDConfig**：`id`, `trunk_id`（空→`*`）, `did`, `target_type`=`queue`\|`ivr`\|`reject`, `target_id?`（reject 须空）。唯一键：`(trunk_id, did)`。

#### IVR 已发布 `/ivr/flows`

**草稿由业务系统自行管理**；Switch 只存已校验的有效 payload，供话务使用。

| 方法 | 路径 | Body |
|------|------|------|
| GET | `/ivr/flows` / `/ivr/flows/{flowId}` | — |
| POST/PUT | `/ivr/flows` / `/ivr/flows/{flowId}` | `id`/`flow_id?`, `payload_json` 或 `payload` |
| DELETE | `/ivr/flows/{flowId}` | 有 DID 引用则拒绝 |

响应 `IVRPublishedView`：`flow_id`, `version`, `payload_json`。每次 Upsert 对该 flow **version+1** 并激活新配置包。

#### 通用按键采集节点

Switch 使用 `collect_input`，接收单个原始 DTMF，不内置满意度、分值范围或报表规则。例如：

```json
{
  "start": "input",
  "nodes": {
    "input": {
      "type": "collect_input",
      "result_key": "reference",
      "accepted_digits": "0123456789*#",
      "timeout_sec": 8,
      "next": "end",
      "default": "end"
    },
    "end": { "type": "hangup" }
  }
}
```

`result_key` 必须非空；`accepted_digits` 仅允许 `0–9`、`*`、`#`；`next` 与 `default` 必须显式配置。`file` 可选，用于播放提示素材。合法输入产生 `ivr.input_collected` 后走 `next`；不允许的按键忽略，节点超时走 `default`，超时不产生采集事件。

open-call 草稿可保留业务节点 `csat`，发布与整包导入时编译为 `collect_input`，使用 `result_key=csat`、`accepted_digits=12345`；评分持久化与统计由 open-call 完成。

#### 本次边界调整的升级

这是接口替换：Switch 移除 `/calls/{callId}/survey`、`csat` 节点及 `call.csat_scored` 事件。浏览器继续使用 open-call 的 `/api/v1/calls/{callId}/survey`。已有评分记录保留，无数据库表结构变更。

1. 升级前导出配置并备份两侧数据库，排空在途通话及旧事件投递，暂停配置修改。
2. 检查所有评价草稿及导出包中的已发布评价节点，补齐 `next` 和 `default`；结束流程使用显式 `hangup` 节点。
3. 同时升级 open-call、open-switch 与前端，通过 open-call 导入完整配置包。导入会把所有旧业务评价节点编译为通用节点，并由 Switch 整包校验和激活；多个旧流程应一起转换，避免增量发布时其他旧节点仍阻止整包校验。直接对接 Switch 的业务系统须自行编译。
4. 确认激活配置中没有 `type=csat`，再恢复话务。验证按 `1–5` 记分、其他键忽略、超时结束、坐席释放，以及新外呼冲突时原通话保持不变。

旧快照不提供运行时兼容分支；没有转换的旧节点会明确报错，需重新编译发布。

### 3.3 坐席签入

| 方法 | 路径 | Body | 响应 |
|------|------|------|------|
| POST | `/agents/{agentId}/check-in` | `{ queue_ids: string[] }` | `AgentSession` |
| POST | `/agents/{agentId}/check-out` | — | — |
| PUT | `/agents/{agentId}/presence` | `{ state, reason? }` | `AgentSession` |
| GET | `/agents/{agentId}/session` | — | `AgentSession` |

**AgentSession**：`agent_id`, `state`（idle/busy/ringing/on_call/acw 等）, `busy_reason?`, `current_call_id?`, `queue_ids`。

---

## 4. 事件回调（Switch → 业务系统）

### 4.1 投递约定

| 项 | 约定 |
|----|------|
| 方法 | `POST` 到 Switch 配置的 `events_callback_url` |
| 鉴权 | 无 |
| 超时 | 约 10s |
| 重试 | 失败指数退避，最多约 12 次 |
| 成功 Ack | HTTP 2xx，且 body（或 `data` 内）`accepted=true`；`event_id` 为空或等于本次事件 `id` |

建议应答：

```json
{ "code": "OK", "data": { "accepted": true, "event_id": 123 } }
```

按全局事件 `id` 做幂等游标；重复投递应安全。

### 4.2 Event 顶层字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | **全局单调**事件 ID |
| `call_id` | string | 通话 ID（部分坐席事件可空） |
| `seq` | int64 | **单通话内**序号，从 1 递增 |
| `version` | int64 | 关联通话版本 |
| `command_id` | string? | 关联幂等命令 |
| `agent_id` | string | 目标坐席 |
| `target_only` | bool | `true` 时勿向访客广播 |
| `type` | string | 事件类型 |
| `payload` | object | 类型相关载荷 |
| `created_at` | time | UTC |

对账：`GET /switch/v1/events?after_id=&limit=` → `{ items: Event[] }`。

### 4.3 主要事件类型

#### 生命周期

| type | 典型 payload | 说明 |
|------|--------------|------|
| `call.created` | `direction` | 创建 |
| `call.state_changed` | `state`, `queue_id`, `version` | FSM |
| `leg.incoming` | `leg_id`, `role` | 客户腿 |
| `call.ringing` / `leg.ringing` | `queue_id`, `caller`, `agent_id`… | 振铃 |
| `call.answered` | `agent_id` | 接通 |
| `call.hold` / `call.unhold` | — | 保持 |
| `call.ended` | — | 结束 |
| `call.transferring` / `call.consulting` / `call.transferred` | — | 转接 |
| `call.voicemail` / `call.supervisor_listen` / `call.device_failed` | — | 其它 |

#### 排队 / IVR

| type | 说明 |
|------|------|
| `routing.entered_ivr` | 进入 IVR |
| `ivr.prompt` | 提示 |
| `ivr.input_collected` | 原始输入：`call_id`, `agent_id?`, `flow_id`, `flow_version`, `node_id`, `result_key`, `input` |
| `queue.entered` / `queue.position_changed` / `queue.overflowed` | 排队 |
| `acd.agent_reserved` | ACD 预留 |

#### 录音 / CDR（业务侧应落库投影）

| type | payload 要点 |
|------|----------------|
| `recording.notice` / `started` / `stopped` | 录音过程 |
| `recording.failed` | 启动或运行中失败；运行中包含 `recording_id`, `failure_reason`, `status=failed` 及部分文件元数据，通话继续 |
| `recording.saved` | `id`, `call_id`, `file_path`, `media_type`, `started_at`, `ended_at?`, `retain_until?`, `file_size`, `recording_semantics`, `channels`, `duration_samples`, `status`, `failure_reason`, `sample_rate_hz`, `leg_paths` |
| `cdr.updated` | `call_id`, `direction`, `queue_id`, `agent_id`, `caller`, `callee`, `session_type`, `result`（answered/abandoned/failed）, `started_at`, `answered_at?`, `ended_at?`, `video_*`, `screen_share_count`… |

新音频主录采用 `conversation_mono_v1`：完整双方与应记录提示音、单声道 PCM16 WAV、8 kHz，各源计一次；分轨包含同一播放时间轴上的解码/PLC 后声音，晚加入和保持补零。历史文件标为 `legacy`，不覆盖。录音队列上限 5 秒、停录等待上限 5 秒，失败保留部分文件并明确标记。详细规则见 [媒体契约](audio-quality.md)。升级将现有活动宽带配置迁移为新的窄带版本，原配置历史与校验和保留。

#### 业务动作（必须处理）

| type | payload |
|------|---------|
| `business_action.requested` | `call_id`, `action_id`, `node_id`, `action`, `deadline`, `outcomes`（键→下一节点） |
| `business_action.completed` / `expired` | `action_id`, `outcome?` |

对 `requested`：在 deadline 内 `POST .../business-actions/{actionId}/complete` 提交 `outcome`。决策应快于 Dispatcher 超时（约 10s）。

#### 坐席 / Direct / 命令

| type | 说明 |
|------|------|
| `agent.routing_state_changed` | 常带 `target_only=true` |
| `leg.*` / `bridge.*` | Direct 腿与桥接 |
| `command.accepted` / `succeeded` / `failed` | 幂等命令结果 |

---

## 5. 典型时序

### 5.1 Web 呼入（业务系统创建）

```mermaid
sequenceDiagram
  participant Biz as 业务系统
  participant SW as open-switch
  participant Agent as 坐席端

  Biz->>SW: POST /calls/inbound
  Note over SW: IVR或排队 / ACD
  SW-->>Biz: call.ringing
  Biz-->>Agent: 通知振铃
  Agent->>Biz: 接听
  Biz->>SW: POST /calls/{id}/answer
  SW-->>Biz: call.answered
  Note over Biz,SW: WebRTC offer/answer/ice
  Biz->>SW: hangup
  SW-->>Biz: recording.saved / cdr.updated / call.ended
```

### 5.2 SIP/DID 呼入

Switch 本地解析 DID 后建呼，**不经**业务系统的 inbound API；业务系统只收事件并驱动坐席侧信令/接听。

### 5.3 business_action

```mermaid
sequenceDiagram
  participant SW as open-switch
  participant Biz as 业务系统
  SW->>Biz: business_action.requested
  Biz->>Biz: 业务决策
  Biz->>SW: CompleteBusinessAction(outcome)
  Note over SW: 进入 IVR 下一节点
```

### 5.4 配置发布

日常：3.2 资源 API（队列/技能/DID/坐席绑定等）每次写即激活新版本；**Switch 为运行时配置真相源**。  
整包：`POST /configuration/versions` → `POST .../activate`（configio 导入/灾备用）。  
open-call 的 `oc_queues` / `oc_did_routes` 等为遗留镜像，访客签发与实时报表直接读 Switch，不再以本地表为准。

---

## 6. 一致性约束（必读）

1. **先激活配置，再呼叫**  
2. **事件全局 `id` 单调**；业务侧游标只前进，乱序晚到应幂等忽略  
3. Switch **先落库再推送**；Ack 失败会重试，业务侧须幂等  
4. 同 `call_id` 命令串行；配合 `expected_version` 与 `Idempotency-Key`  
5. 仅当前 offered 坐席可 Answer  
6. 录音通常在接通后开始；无队列通话默认不录音（`off`），须显式策略才录  
7. `business_action` 须在 deadline（及推送超时）内回填  
8. **单实例部署**：控制面 FSM / 媒体 / SIP registrar 不跨进程；勿多活共享库  
9. 呼入 `config_version` 由 Switch 从激活配置或 DID/队列注入，客户端不可伪造  

联调检查：

- [ ] Switch 已配可达的 `events_callback_url`  
- [ ] 已激活含队列/坐席/DID（如需）的配置  
- [ ] 坐席已 check-in 到目标队列  
- [ ] callback 返回 `accepted:true` 且 `event_id` 匹配  


## 应用音频与原生播放

Switch 提供通用 PCM WebSocket 和按腿原生播放；call 分别实现机器人对话、通知任务。协议、缓冲上限、幂等与迁移见 [应用音频与业务边界](./应用音频与业务边界.md)。

- `GET /switch/v1/calls/{callId}/legs/{legId}/media`：已接通 agent/application 腿的 PCM 会话，支持 duplex/sendonly/recvonly。
- `POST/GET/DELETE .../legs/{legId}/playbacks[/{playbackId}]`：原生素材播放、状态查询与精确停止。
- `POST /switch/v1/calls` + `POST .../legs/sip`：不依赖坐席的通用出局；单腿 SIP 接通后 Call 进入 active。

Switch 不再暴露语音通知专用接口或模式。通知通过 call 的 `POST /api/v1/calls/voice-notifications` 提交，HTTP 202 返回持久化业务任务；发起人不占坐席，任务不依赖 AI 配置。此接口返回类型发生变更，需协同升级前后端并排空旧任务。
