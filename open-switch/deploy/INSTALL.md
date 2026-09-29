# open-switch 安装

负责通话控制、SIP/WebRTC、录音文件及持久化事件流。Switch API（`/switch/v2`）只对持应用密钥的业务服务开放；录音目录必须可写。

完整联调和生产门槛见 [SIP 上线验收](../../docs/sip-production-acceptance.md)。交付仍使用二进制与 systemd，不要求 Docker。

## 构建和配置

在 `open-switch/` 执行 `go build -o open-switch ./cmd/open-switch`，复制 `deploy/config.example.yml` 为部署配置。安装 PostgreSQL 16+；生产环境建议创建独立数据库 `openvoip_switch` 和最小权限运行账号。本机联调可与 open-call 共用数据库，但要保持 `oc_*` / `os_*` 表归属，不做跨服务直连查询。

目录：

```text
/opt/open-switch/
  open-switch
  config.yml
  data/
```

配置 `integration.register_token`、数据库与外部地址。业务系统通过 `POST /switch/v2/integrations/register` 登记并获取 `secret`（见 [switch-standalone-runbook](../../docs/switch-standalone-runbook.md)）。默认 HTTP 端口为 8082。服务只读取显式 `-config` 文件，不使用环境变量覆盖部署配置。

启动时自动执行 `internal/store/migrate/sql` 内嵌迁移并记录 `os_schema_migrations`；迁移失败会停止启动。旧版共用 `schema_migrations` 不再参与判断。新版本增加边界/恢复字段；升级前停止派单、排空通话、备份数据库和录音目录。旧单体无前缀表须制定单独数据迁移方案，禁止直接复用后假设自动兼容。

## systemd

将 [open-switch.service](open-switch.service) 安装至 `/etc/systemd/system/`。创建 `openvoip` 服务用户，为其授权运行目录（交换服务另需录音目录写权限），配置文件权限设为 0640。执行：

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now open-switch
sudo journalctl -u open-switch -f
```

两个服务各用自己的二进制、配置和工作目录；不要继续安装旧 `open-voip.service`。

## 网络与验证

前端静态文件由 Nginx 托管，浏览器 API/WS 只代理 open-call。HTTPS 是非 localhost 浏览器麦克风访问的前提。**不要**将 `/switch/v2` 暴露到公网；仅允许业务服务（如 open-call）内网访问 Switch HTTP；跨机器可使用内部 TLS。

SIP 配置和每个设备的密码只在 open-switch；SIP UDP 5060 及 RTP 范围仅对设备/SBC 网络开放。示例 WebRTC 10000–20000 与 SIP 20002–20100 不重叠。公网 NAT、TLS SIP、TURN 均须现场验证。

`/health` 是存活探针。open-call `/api/v1/status` 检查业务库和交换服务查询失败时返回 503。先按验收文档跑真实库回归、REGISTER/INVITE 双向音频、异常挂机、进程重启和备份恢复，再进行容量验收。

备份必须包括：两份 PostgreSQL 数据、两份配置、open-switch 的录音和 prompts。业务侧话单/录音元数据依赖 Switch **HTTP callback** 至 open-call；应监控 `os_integrator_event_deliveries` 重试积压与 open-call 出站/Webhook 积压。
