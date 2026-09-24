# 当前项目与需求、技术设计的偏差及生产可用性评审

> 修订说明（2026-09-24）：本文是修复前的评审快照。open-call 已进一步加入可撤销认证会话、一次性访客令牌、持久 Webhook/死信、完整配置预检恢复、资源引用校验、数据库约束、结构化 ACW、质检校验、流式 CDR 导出和受保护运维状态；SIP/RTP 配置已移至 open-switch。迁移已在当前 PostgreSQL 配置上连续执行两次。当前验证范围及未通过的生产门槛统一见 [上线验收](sip-production-acceptance.md)，不要继续将本报告的全部问题视为未修复，也不要把代码修复视为真实话路或容量认证。

评审日期：2026-09-21。对象：当前工作区，包含正在进行的单体拆分、目录迁移及未提交修改；不代表某个已发布版本。

**结论：当前版本不应直接作为生产呼叫中心上线。** 项目已经具备较广的功能实现基础，但拆分后的鉴权、来电分配、查询、话单和录音收尾存在确定性缺陷，连文档定义的小型内网 P0 主链路也不能判定验收通过。适合继续开发及隔离环境联调；修复关键问题并完成真实环境验收后，才适合小范围试点。

本报告区分“实际复现”“静态确认”和“尚未验证”。不把接口存在等同于功能验收，不根据文件数量估算完成百分比。

## 1. 按什么标准评价

[需求文件](C:/Users/yaojie/Desktop/open-voip/docs/requirements.md:10)限定了 5～30 坐席、内网、单机、可接受计划停机；P0 要求至少 10 路语音，P1 要求至少 5 路 720p 视频，语音典型端到端延迟低于 150ms。

- **需求符合性**：应以这个小规模内网范围验收。没有 Kubernetes、Redis、多租户、集群或异地双活，不构成本次范围内的缺陷。
- **生产可用性**：即使只有 5 个坐席，也需要正确的鉴权、稳定的呼叫状态、可信的话单与录音、可恢复的故障处理及可执行的安装备份流程。
- **通用企业级平台**：多节点容灾、大规模并发、完整运营商互通与运营保障超出当前需求范围，现有实现也没有提供足够验证证据。

## 2. 已实现的基础

项目并非仅有界面或空接口：存在用户与 JWT/Argon2 认证、角色路由、坐席及队列管理、ACD、IVR 发布和运行时、呼叫状态控制、Pion WebRTC、SIP 相关实现、录音、CDR、报表、Webhook、三端 Lit 页面、SQL 迁移和 CI。

当前分工是 open-call 负责业务与浏览器入口，open-switch 负责呼叫控制和媒体，两者通过 HTTP 互调。主要问题是这些模块的组合尚不可靠，尤其是从单体改为双进程以后。

## 3. 阻止生产上线的具体问题

以下“阻断/高”是本次上线风险级别，不等同于原需求文档的 P0/P1 功能排期。

### F01｜阻断：通话代理绕过用户鉴权并保留外部身份头

**证据：实际复现代理行为，并单独验证 Switch 接受同样的伪造身份。**

[组合根](C:/Users/yaojie/Desktop/open-voip/open-call/internal/app/run.go:132)将 WrapSwitchBFF 放在带认证的 API router 外面。[代理](C:/Users/yaojie/Desktop/open-voip/open-call/internal/app/http/bff.go:17)提前截获通话路径，替换 Authorization 为服务间密钥，却没有先认证用户，也没有清除外部传入的 X-Principal。

结果有两个方向：

- 普通客户端只提供合法 JWT 时，没有经过中间件生成主体，Switch 接听/信令等接口会拒绝请求。
- 未登录请求可以携带伪造的主体头，经代理获得服务间凭据，进入 Switch 的信任边界。验证中 Switch 接受了伪造管理员主体并返回通话视图。

关联 PLAT-02/03、MEDIA-02、QUEUE-03。应先认证再代理，主动清除外部身份头，仅由服务端构造主体；服务调用身份和最终用户身份应明确区分。跨域中间件也被当前代理提前绕过，应同时验证 OPTIONS 和跨域调用。

### F02｜阻断：ACD 使用旧表名，新库无法正常执行分配查询

**证据：静态确认 SQL 与迁移不一致；没有连接真实 PostgreSQL 执行。**

[RequestAgent](C:/Users/yaojie/Desktop/open-voip/open-call/internal/layers/biz/queue/service.go:333)仍然 JOIN agents、agent_session_queues、agent_skills，并使用 agent_sessions 列前缀；[模型](C:/Users/yaojie/Desktop/open-voip/open-call/internal/store/models/agent.go:73)和[迁移](C:/Users/yaojie/Desktop/open-voip/open-call/internal/store/migrate/sql/000001_init.sql:96)使用 oc_ 前缀。

按照当前迁移建立的新库，不存在这些旧关联表；即使旧表碰巧残留，查询中的主表列前缀仍与 GORM 生成的主表不一致。核心“入队→选人→振铃”不能正常完成。

另外，技能筛选使用 IN，表达“至少命中一项技能”，没有实现技术设计要求的“具备全部所需技能”。关联 QUEUE-02/03、AGENT-06/07。应在真实 PostgreSQL 上验证普通分配、多个技能、视频过滤以及并发抢占。

### F03｜高：服务间主体契约不一致，内部通话查询及 WS 接听受阻

**证据：静态确认客户端和服务端代码。**

[Switch 客户端 GetCall](C:/Users/yaojie/Desktop/open-voip/open-call/internal/integration/switchapi/client.go:76)只带服务密钥，不带主体；Switch 的[authorizeCall](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/app/http/call_handlers.go:153)要求有效主体。因此 guest 服务查询通话腿时会失败，customerLeg 又把错误转为空 leg_id/state，可能使访客无法正确建立媒体。

客户端 principalAgent 使用 snake_case 的 agent_id，且没有 UserID/GuestID；[接收结构](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/authctx/principal.go:5)没有对应 JSON tag，[主体有效性检查](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/app/http/middleware/principal.go:20)又要求 UserID 或 GuestID 非空。WS 接听/拒接并不能靠这个内部客户端绕过 F01 的正常用户故障。

应统一版本化 DTO，并为服务身份可执行的接口建立显式授权策略，不能简单地取消所有主体校验。

### F04｜高：班长操作与通话归属的授权不完整

**证据：静态确认。**

Switch 的[强制签出](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/app/http/switch_handlers.go:218)只检查存在主体；[监听](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/app/http/switch_handlers.go:234)只要求主体带 AgentID，没有验证 admin/supervisor 角色。

authorizeCall 还允许一般坐席访问 queued/ringing 通话，并允许未绑定 call_id 的访客主体通过归属判断。媒体接口只检查 call 级访问，没有验证 leg 是否属于调用方。即使修复 F01，仍应补齐角色、通话、媒体腿三个层面的权限测试。关联 PLAT-03、CALL-04、AGENT-05。

### F05｜高：已接通 CDR 的坐席 ID 丢失

**证据：实际复现。**

[Answer](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/layers/control/service.go:224)在接听后清空 offeredAgent，而[cdrUpsert](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/layers/control/service.go:636)仍以 offeredAgent 填充 AgentID。

复现“入队→ag1 接听→正常挂断”，最终 CDRWriteRequest.AgentID 为空。这会影响坐席归属、按人统计、质检关联。关联 CDR-01、RPT-*。应分别保存被邀请坐席和已接通坐席，并定义转接后的话单归属。

### F06｜高：录音收尾顺序错误，最终元数据无法更新

**证据：实际复现真实媒体服务的资源生命周期。**

[Hangup](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/layers/control/service.go:336)先 CloseRoom，随后 stopRecording。CloseRoom 会[删除录音索引](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/layers/media/service.go:235)，之后 stopRecording 查询 RecordingInfo 得到“录音不存在”。

录音文件可能已经关闭并保存，但元数据仍停留在开始录制时的快照；时长、文件大小以及成功封装后的 WebM 路径无法可靠更新。不能将此问题描述成所有录音文件必然丢失。

应先完成录音收尾并保存最终元数据，再释放对应索引，或者让关闭操作返回最终元数据。关联 REC-01/02、ADM-03。

### F07｜高：通话小结和质检 API 被错误代理

**证据：通话小结路由实际复现；质检为相同规则的静态确认。**

[shouldProxyToSwitch](C:/Users/yaojie/Desktop/open-voip/open-call/internal/app/http/bff.go:59)将所有 /api/v1/calls/** 转发到 Switch。但是 wrap-up 和 qa-marks 是 open-call 本地业务路由，Switch 没有对应实现。

应使用明确的通话控制/媒体路径白名单，保留本地业务端点。关联 UI-A-07、QA-01。

### F08｜高：实时监控与拆分后的数据归属脱节

**证据：静态确认。**

[Live 报表](C:/Users/yaojie/Desktop/open-voip/open-call/internal/layers/biz/report/service.go:63)查询本地 models.Call，即 oc_calls；但 open-call 的迁移没有建立此表，真实通话保存在 open-switch 的 os_calls，Platform 事件处理也没有同步本地通话读模型。多数报表查询错误被忽略，可能返回看似正常的 0。

同时[StatusProvider 注入](C:/Users/yaojie/Desktop/open-voip/open-call/internal/app/run.go:123)没有 ActiveCalls 函数，因此 /status 的 active_calls 保持默认 0；/health 仅返回 OK，不能表示 Switch、媒体或数据库就绪。

应明确实时状态查询由 Switch 提供，或建立可校验、可重建的本地读模型；健康检查区分进程存活和依赖就绪。关联 RPT-01、PLAT-06、DEPLOY-08。

### F09｜高：跨服务失败后没有可靠补偿与重启收敛

**证据：静态确认当前实现；未执行进程故障演练。**

[Platform client](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/integration/platform/client.go:40)执行单次 HTTP 调用。接听/挂断多处忽略 CDR、坐席状态和事件写入错误；[挂断](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/layers/control/service.go:355)之后还删除运行时通话。业务服务短暂故障可能留下缺失话单、未结束话单、卡在 on_call/ringing 的坐席。

[NewService](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/layers/control/service.go:75)从空内存 map 启动；当前启动链未见恢复、终结数据库残留通话并同步释放坐席的流程。重启后旧呼叫可能仍在数据库显示 active，但无法按正常运行时路径挂断。

WS 断开只移除连接映射，没有离线宽限期及坐席状态收敛；前端重连也没有完整状态补拉协议。页面关闭后坐席仍可能被分配来电。

单机允许维护停机不等于允许状态永久残留。可采用持久化待办/补偿记录、幂等回调、启动清理和重连对账；不必为了这个规模引入大型消息平台。

### F10｜高：状态机并发与重复操作保护不足

**证据：静态风险，未做竞争检测或并发压测，不能量化出现概率。**

Answer、Hangup、tick 等在锁内取 runtimeCall 指针后，仍在锁外读可变字段；transition 直接覆盖目标状态，没有统一验证合法迁移或版本。[transition](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/layers/control/service.go:604)先改内存再写库，失败不回滚。

因此接听与超时、接听与挂断等交错执行存在状态不一致风险。另外[重复签入](C:/Users/yaojie/Desktop/open-voip/open-call/internal/layers/biz/agent/service.go:147)会无条件将现有会话设置为 idle，没有先拒绝 on_call/ringing，会破坏单坐席容量约束。

应实现每通呼叫的串行化/版本校验、幂等命令及明确失败处理；补充真实并发测试和 race 检测。关联 AGENT-03、QUEUE-02、技术设计 §3.2/3.3。

## 4. 需求与设计的其他偏差

| 项目 | 文档与实现对照 | 判断 |
|---|---|---|
| 部署架构 | technical-design §1/2/12 仍声明单进程、单二进制；README 与对接说明已经改为双进程、双库 | 架构演进本身合理，但文档基线未统一 |
| 安装服务 | 两套 deploy/open-voip.service 仍使用同一个 /opt/open-voip/open-voip 和 config.yml | 按当前构建出的 open-call/open-switch 直接照说明部署不能完成交付；需两份服务及配置 |
| 无限流、无主机 CPU 指标、人工清理录音、同步 Webhook 重试 | technical-design §2.10 明确将原需求降级 | 是已记录折中，不应全部算作开发漏做；使用范围扩大时需重新审定 |
| 屏幕共享 | 技术设计要求独立 screen track；shared/webrtc.js 的 startScreenShare 替换摄像头 track | 简化实现；可满足基础共享意图，但没有实现设计中的独立双轨 |
| 升降视频 | 前端关闭原 PeerConnection 后重建媒体，而非在原连接连续重协商 | 有瞬断和设备重申请风险；未实测中断时长 |
| 纯内网 ICE | 后端可配置 STUN，但前端硬编码 Google STUN | 不等于无公网一定无法直连；仍偏离可省略/本机配置意图，需断网验收 |
| H.264 与多方媒体 | 注册 H.264，但输出视频轨固定 VP8；多人 RTP 共用单个输出轨，录制也把多源送入同一 writer | 不能凭 codec/接口存在宣布 Safari、多方通话、双向录音质量已验收；需多源轨设计审查及实测 |
| 容量与延迟 | 需求明确 10 路语音、5 路 720p、典型语音 <150ms | 本次没有发现可支撑这些指标的压测与端到端测量结果，状态为未验证 |
| 工程文档 | implementation-plan 仍引用 app/、旧入口和 AutoMigrate；当前使用双模块和 SQL 迁移 | “Phase 2/3 主路径已落地”不能作为当前版本验收证明 |

媒体评估位置：[媒体转发](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/layers/media/service.go:603)、[录制写入](C:/Users/yaojie/Desktop/open-voip/open-switch/internal/layers/media/service.go:748)、[前端共享](C:/Users/yaojie/Desktop/open-voip/open-call-web/shared/webrtc.js:135)。这些静态风险没有被本次验证为真实通话音质故障。

## 5. 本次验证和限制

| 验证 | 结果 | 能说明什么 |
|---|---|---|
| 两个 Go 模块原有 go test ./... | 通过，部分使用缓存 | 现有自动化用例通过，不能证明跨服务业务正确 |
| 两个模块 go vet ./... | 通过 | 基础静态检查通过 |
| 前端 npm run build | 通过 | 三端可生成发布资源；不代表浏览器通话验收 |
| 两个模块数据库集成迁移测试，强制重新执行 | 均 SKIP：未设置 OPEN_VOIP_TEST_DSN | 没有完成真实 PostgreSQL 集成验证 |
| 临时补充安全/路由测试 | 安全预期断言失败；小结本地路由断言失败；Switch 单独验证接受伪造主体 | 复现 F01、F07 |
| 临时补充完整接听挂断话单测试 | AgentID 实际为空 | 复现 F05 |
| 临时补充真实媒体录音生命周期测试 | RecordingInfo 返回“录音不存在” | 复现 F06 |

补充测试在隔离的本地 HTTP 测试服务器、假持久层和临时录音目录中执行，没有操作运行中的用户通话。为避免评审遗留失败测试影响项目，本次临时测试文件已移除，业务源码未修改；保留本报告。

前端依赖从本机缓存安装，构建因沙箱子进程限制先失败，获准在沙箱外重跑后通过。未验证实际浏览器双向声音/图像、真实 SIP 运营商、跨 VLAN/TURN、音画同步、连续负载、异常网络、磁盘满或备份恢复。未执行完整 golangci-lint/OpenAPI lint，也不把未执行的检查标成通过。

现有测试薄弱点包括：open-call HTTP router 测试主要验证 health；ACD 测试验证录音策略而非真实 SQL 分配；Switch 的多数状态机测试依赖 fake media、fake ACD、fake persistence，没有覆盖 HTTP 互通及真实数据库。CI 的两个后端作业分别运行，也不是一个完整的双服务通话验收环境。数据库不可达时集成测试还会 Skip，建议 CI 将必需的数据库不可达视为失败。

## 6. 达到可试生产的建议验收门槛

1. **先修复基本正确性和访问边界**：F01～F08，统一主体 DTO、代理白名单、数据库查询、话单归属和录音收尾；从全新双库部署完整走通两浏览器呼入。
2. **再处理异常与并发**：服务超时、进程重启、重复命令、接听/挂断/超时竞争、页面断开、坐席重复签入；明确失败话单和遗留状态如何清理。
3. **补充真实集成验收**：访客入队、振铃、接听、双向媒体、拒接重派、超时、转接、挂断、CDR、录音回放及权限隔离。测试要经过真正的 BFF、Switch、Platform API 和 PostgreSQL。
4. **验证原需求指标**：10 路语音、5 路 720p，记录 CPU/内存/带宽、成功率、延迟和音画质量；建议增加 24～72 小时连续运行及故障注入，这是本评审建议，不是原需求已经承诺的测试时长。
5. **完成部署与恢复演练**：双服务安装、配置和密钥管理、TLS、防火墙、录音目录权限、双库与文件备份、恢复、升级回滚、就绪检查和告警。录音下载依赖 open-call 访问 Switch 写入的文件，单机上需明确共享目录和用户权限。

满足上述门槛后，可以按原需求目标开展小型内网试生产；是否扩大到公网、运营商线路或更高可用性，应另立验收范围。当前不建议以增加业务功能为第一优先级，优先完成已有核心链路的正确性和稳定性。
