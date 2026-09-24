# 双服务与 SIP 上线验收

更新：2026-09-22。本文记录当前代码边界和验收条件，不表示已取得生产验收结论。历史差距见 [2026-09-21 评审](production-readiness-review-2026-09-21.md)。

## 服务与数据归属

| 对象 | 唯一负责服务 | 另一服务如何使用 |
|---|---|---|
| 用户、JWT、角色、坐席、队列、技能、ACD、DID、IVR 配置 | open-call | Platform API；禁止读取业务数据库 |
| 坐席的 `terminal_type` / `sip_username` | open-call | AgentInfo 返回终端绑定；不传设备密码 |
| SIP 设备密码、注册 Contact、Digest nonce、ACL、中继、RTP | open-switch | open-call 只保存绑定账号 |
| 通话 FSM、通话腿、当前媒体与 SIP dialog | open-switch | Switch API；实时查询不能读取 `oc_calls` |
| CDR、录音元数据、小结、质检、报表 | open-call | Switch 回调；话单按 call_id 更新 |
| 录音文件、IVR 音频文件 | open-switch | 下载/删除走内部文件接口；无需共享文件系统 |

浏览器只访问 open-call。BFF 首先验证用户令牌，删除外部 `X-Principal`，再用已验证身份生成 snake_case 身份头和服务密钥。小结、质检留在 open-call；内部呼入、内部运行时和文件接口不经过浏览器代理。普通坐席只能控制自己的通话与媒体腿；班长操作要求 supervisor/admin。访客必须绑定具体 call_id。

部署可以同机，也可以分机。生产环境建议两个服务使用独立 PostgreSQL 数据库及最小权限账号；本机联调允许共用一个数据库，因为业务表与交换表分别使用 `oc_*`、`os_*` 前缀，迁移记录也分别写入 `oc_schema_migrations`、`os_schema_migrations`。旧版共用的 `schema_migrations` 不再参与判断；升级前备份数据库，确认两个服务的迁移均已成功执行。当前运行时和 SIP 注册均为单实例，不能用多个 open-switch 副本共享同一话务库。

## 两类 SIP 接入

### 中继 / PBX / 语音网关

在 open-switch 的 `sip.trunks` 配置对端、凭证和源 IP 白名单，在 open-call 管理 DID → 队列。中继入呼由 open-switch 接收，交给 open-call 查 DID 和选人；建立 SIP 会话后可进入排队/IVR，不再等待浏览器媒体才向中继应答。注意：入呼进入系统即应答，运营商可能从此开始计费。

外呼请求返回通话视图后异步振铃，接到 SIP 成功响应才记录 answered_at 并开始接通流程。显式指定不存在的中继会失败，不回退到另一条中继；跨主机的 SIP 3xx 重定向不会自动跟随。

### SIP 话机 / 软电话坐席

1. 管理端创建用户时选择「SIP 话机 / 软电话」。API 同样支持 `terminal_type: "sip"`，`sip_username` 必须等于 `extension`，`video_capable` 必须为 false。修改绑定前先签出。
2. open-switch 设置 `sip.enabled: true`、`local_registrar: true`，为每个分机配置独立 `devices` 凭证和 ACL。设备密码至少 12 字符；旧 `registrar_password` 共用密码不再接受。
3. 话机注册服务器填写 open-switch 的地址/端口；认证用户名填写分机号，域填写 `sip.local_domain`（未设置时使用 external_ip）；音频使用 PCMU/PCMA。
4. 在坐席工作台登录并签入已绑定队列。SIP 注册与业务签入是两件事：只有注册不能获得队列派单，只有签入而话机未注册也无法接听。
5. ACD 派单后 open-switch 向注册 Contact 发 INVITE；在话机上接听。浏览器接听按钮不能替代 SIP 的接听响应。SIP 坐席从话机拨号；浏览器显示状态、控制保持/挂断并填写小结。
6. 设备拒接/超时/不可达后坐席置忙（`sip_unavailable`），防止反复派给离线话机。修复设备后主动示闲。

示例（密码和地址必须替换）：

```yaml
sip:
  enabled: true
  listen: "0.0.0.0:5060"
  transport: udp
  external_ip: "192.168.10.20"
  local_domain: "cc.internal"
  rtp_port_min: 20002
  rtp_port_max: 20100
  local_registrar: true
  devices:
    - username: "1001"
      password: "REPLACE-WITH-UNIQUE-RANDOM-PASSWORD"
      allowed_cidrs: ["192.168.10.0/24"]
  trunks:
    - id: pbx
      host: "192.168.10.30"
      port: 5060
      register: false
      from_user: "8000"
      codecs: [PCMU, PCMA]
      allowed_cidrs: ["192.168.10.30/32"]
```

两条 SIP 腿分别保存 dialog/RTP，客户中继与坐席话机不会覆盖对方。RTP 只接受 SDP 中协商的来源 IP；允许同一 IP 的对称 RTP 端口更新。对端 NAT 后通告错误私网 IP 时，应修复 SBC/设备 SDP，不依赖无条件接受任意 RTP 来源。

## 状态、失败与恢复

- 同一通话的命令串行执行；同一坐席重复接听幂等。ACD 使用业务数据库行锁和 call_id 锁，重试不重复占用坐席，技能匹配要求满足全部技能。
- 坐席状态回调携带 call_id；迟到的释放通知不能释放另一通话占用的坐席。WebRTC 最后一条工作台连接断开时，空闲坐席转忙；SIP 坐席不因工作台断线停止话机接听。
- `GET /api/v1/agents/me/calls` 返回当前坐席的权威通话快照。工作台连接成功/重连以及接听事件后重新同步。事件不是持久消息日志；第三方客户端也必须重新查询快照。
- CDR、录音元数据、通话坐席释放写入 `os_platform_outbox` 后投递。按序重试，成功才删除；重复 CDR/录音写入按 ID 更新。事件/Webhook 不在该持久队列内，仍不保证最终送达。
- open-switch 启动先扫描遗留未结束通话，结束已经失去媒体的会话并补最终话单/释放坐席。**不恢复已经中断的音频**。停机前停止接入新流量，再结束存量通话。
- `/health` 仅代表 HTTP 存活；open-call `/api/v1/status` 会检查业务库和交换服务查询，依赖失败返回 503。不要将 `/health` 成功视为可以呼叫。
- G.711 录音采用有界缓冲持续写 WAV；正常结束后写最终长度和结束时间，再保存元数据、关闭房间。下载和清理只允许交换服务录音根目录下符合通话 ID/录音 ID 命名的文件。

outbox 是单工作线程顺序投递；毒消息会阻塞后续消息，不能静默丢弃。应监控积压并查 `last_error`，修复业务接口/数据后让它继续重试。监控查询：

```sql
SELECT count(*), min(created_at), max(attempts) FROM os_platform_outbox;
SELECT id, path, created_at, attempts, last_error
FROM os_platform_outbox ORDER BY id LIMIT 20;
```

## 部署和升级

使用 [open-call 安装说明](../open-call/deploy/INSTALL.md) 与 [open-switch 安装说明](../open-switch/deploy/INSTALL.md)。交付两个二进制和两份配置，分别使用 `open-call.service`、`open-switch.service`；前端只连接 open-call。升级两个服务及前端为同一版本。

先停止派单并排空通话，备份两份数据库、两份配置和交换服务录音/提示音目录，再升级。新增业务迁移包含终端绑定、占用 call_id 和待签出字段；新增交换迁移包含回调队列和重启收尾字段。迁移失败时进程不能启动。旧无前缀单体库不是自动迁移来源，不要直接改 DSN 复用旧库。

替换示例中的集成密钥、JWT 密钥、数据库密码和 bootstrap 密码。对外 Nginx 仅公开 `/api/v1` 和静态前端；`/platform/v1` 仅允许交换服务网络来源；`/switch/v1` 仅允许业务服务来源。分机 ACL 和中继 ACL 单独配置。SIP RTP 与 WebRTC UDP 端口范围不能重叠；跨 NAT 配置内部 STUN/TURN。示例不默认依赖公网 Google STUN。

配置导出不是完整灾备：当前 import 仍只恢复队列/技能/DID；用户密码、完整绑定关系和通话数据应使用数据库备份恢复。不要用配置 JSON 替代 pg_dump/恢复演练。

## 验收矩阵与剩余门槛

| 检查 | 本轮状态 | 上线要求 |
|---|---|---|
| BFF 伪造身份、访客越权、坐席操作别人媒体腿 | 自动回归通过 | 保留回归 |
| SIP 独立账号 Digest、错误凭证/URI、nonce 重放 | 自动回归通过 | 真机兼容性复测 |
| 实际 UDP REGISTER → INVITE → 200 → ACK | 本机模拟设备测试通过 | 指定话机/软电话实测 |
| 双 SIP 腿 RTP 转发、来源约束、保持、重复 DTMF | 本机 UDP 回归通过 | 双向音频、丢包、NAT、长通话复测 |
| 并发接听/挂断、重启收尾、话单坐席归属 | 自动回归通过 | 配合真实库断电/重启演练 |
| 迁移、ACD SQL/全部技能/幂等、outbox 恢复 | 测试需 PostgreSQL；本机未执行 | 两个隔离测试库实跑，不能跳过 |
| 10 路语音、30 坐席、5 路视频、延迟/抖动 | 未压测 | 按需求测量并保存报告 |
| 运营商中继、设备拨出、DID 入呼、IVR、转接 | 无现场环境，未完整联调 | 逐项实呼，不以接口 200 代替音频验收 |
| TLS SIP、复杂 NAT、跨 VLAN TURN | 未验证 | 在实际网络验证信令与双向媒体 |

当前仍有明确的软件边界需要后续完善：WebRTC 多方 Opus 音频混音/合成录像、Webhook 持久投递、完整配置恢复；SIP 多方会议媒体混音与咨询转失败回退也需专项验收和补强。上述未完成项不应标记为生产就绪。新版本可进入受控的语音联调，**不能仅凭本机测试直接批准生产上线**。

测试方式：分别在两个 Go 模块执行 `go test ./...`、`go vet ./...`；有 C 编译器的环境增加 `go test -race ./...`。设置 `OPEN_VOIP_TEST_DSN` 时必须指向各自可写的隔离测试库；配置了不可达数据库会直接失败，不再悄悄跳过。前端执行 `npm ci`、`npm run build`。
