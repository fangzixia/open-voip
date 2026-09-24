# open-call 安装

负责账号、队列、技能、ACD、DID、话单元数据、WebSocket 与 BFF。前端连接本服务；不要在此配置 SIP 设备密码。

完整联调和生产门槛见 [SIP 上线验收](../../docs/sip-production-acceptance.md)。交付仍使用二进制与 systemd，不要求 Docker。

## 构建和配置

在 `open-call/` 执行 `go build -o open-call ./cmd/open-call`，复制 `deploy/config.example.yml` 为部署配置。安装 PostgreSQL 16+；生产环境建议创建独立数据库 `openvoip_call` 和最小权限运行账号。本机联调可与 open-switch 共用数据库，但要保持 `oc_*` / `os_*` 表归属，不做跨服务直连查询。

目录：

```text
/opt/open-call/
  open-call
  config.yml
  data/
```

替换集成密钥（两端一致）、数据库密码和外部地址。open-call 另需替换 JWT、bootstrap 密码。`integration` 中的对端 URL 必须可互访。默认 HTTP 端口为 8080。服务只读取显式 `-config` 文件，不使用环境变量覆盖部署配置。

启动时自动执行 `internal/store/migrate/sql` 内嵌迁移并记录 `oc_schema_migrations`；迁移失败会停止启动。旧版共用 `schema_migrations` 不再参与判断。新版本增加边界/恢复字段；升级前停止派单、排空通话、备份数据库和录音目录。旧单体无前缀表须制定单独数据迁移方案，禁止直接复用后假设自动兼容。

## systemd

将 [open-call.service](open-call.service) 安装至 `/etc/systemd/system/`。创建 `openvoip` 服务用户，为其授权运行目录（交换服务另需录音目录写权限），配置文件权限设为 0640。执行：

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now open-call
sudo journalctl -u open-call -f
```

两个服务各用自己的二进制、配置和工作目录；不要继续安装旧 `open-voip.service`。

## 网络与验证

前端静态文件由 Nginx 托管，浏览器 API/WS 只代理 open-call。HTTPS 是非 localhost 浏览器麦克风访问的前提。对外反代必须拦截 `/platform/v1`；允许交换服务内网地址访问它。open-switch `/switch/v1` 仅允许 open-call 来源；跨机器可使用内部 TLS。

SIP 配置和每个设备的密码只在 open-switch；SIP UDP 5060 及 RTP 范围仅对设备/SBC 网络开放。示例 WebRTC 10000–20000 与 SIP 20002–20100 不重叠。公网 NAT、TLS SIP、TURN 均须现场验证。

`/health` 与 `/health/live` 是存活探针；`/health/ready` 同时检查业务库和 open-switch，依赖不可用时返回 503。受保护的 `/api/v1/status` 供管理员和主管查看数据库、交换服务、活动通话、WebSocket、Webhook 积压与进程指标，不返回本地目录。

备份必须包括：两份 PostgreSQL 数据、两份配置、open-switch 的录音和 prompts。open-call 配置导出包含用户、坐席、技能、队列、绑定、DID、IVR 发布版本和 Webhook 订阅，可先用 `dry_run=true` 预检再导入；密码摘要和 Webhook 密钥不导出，因此完整灾备仍以 PostgreSQL 物理或逻辑备份为准。监控 Webhook 死信、open-switch outbox 与录音清理失败记录，禁止直接丢弃积压数据。

升级前先备份数据库，在同版本副本上运行迁移两次验证幂等，再切换生产。`bootstrap.enabled` 在生产必须为 `false`；JWT 密钥至少 32 字符，集成密钥至少 16 字符。修改密码、角色或禁用账号会立即撤销现有 REST、刷新令牌和 WebSocket 会话。
