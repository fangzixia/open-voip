# 服务器快速部署与调试

目标主机默认：`8.153.98.134`（可通过环境变量 `DEPLOY_HOST` 覆盖）。

## 目录约定（服务器）

| 路径 | 说明 |
|------|------|
| `/opt/open-voip/src` | 从本机同步的完整源码（便于 SSH 上改代码、看日志） |
| `/opt/open-switch` | 软交换二进制 + `config.yml` + 录音数据 |
| `/opt/open-call` | 呼叫中心二进制 + `config.yml`（进程用户 `openvoip`） |
| `/var/www/open-call-web` | 前端 `npm run build` 产物 |
| `/etc/nginx/conf.d/open-voip.conf` | Nginx 反代 `/api`、`/health` 与静态页 |

**注意：** 部署脚本**不会覆盖**服务器上已有的 `config.yml` 与数据库，只更新二进制与前端。

## 从旧版 Switch 库升级

若服务器上的 `openvoip_switch` 仍含 `application_id` / `os_applications` 等多租户表结构，与当前仓库单租户代码**不兼容**。可选：

1. **重建 Switch 库**（会丢失 Switch 侧运行时/话务数据，业务库 `openvoip_call` 保留）：

在服务器上（需交互确认数据库名）：

```bash
python3 /opt/open-voip/src/scripts/deploy/reset-db.py --service switch
```

2. 登录管理端 `http://<主机>/`，使用 **配置导出 → 再导入**（或调整队列/坐席后保存），将配置重新发布到 Switch，直至 `/health/ready` 中 `switch_ok` 为 true。

3. 若必须保留旧库数据，请勿部署本仓库当前二进制，需继续使用与旧 schema 匹配的历史版本。

## 配置 YAML 字段变更

并确保已安装 `ffmpeg`（`apt install ffmpeg`），否则 open-call 无法启动。

open-switch 的 `recordings` 必须使用 **`audio`** 与 **`video`** 两个独立配置块（目录、告知文案；录像另含 `format` / `ffmpeg_path`）。请对照仓库内 `deploy/config.example.yml` 手工合并服务器上的 `config.yml`。

## SSH 免密（推荐）

本机 `~/.ssh/config` 示例：

```
Host open-voip
  HostName 8.153.98.134
  User root
  IdentityFile ~/.ssh/id_ed25519
  IdentitiesOnly yes
```

首次可把公钥写入服务器：`type $env:USERPROFILE\.ssh\id_ed25519.pub | ssh root@8.153.98.134 "mkdir -p ~/.ssh && cat >> ~/.ssh/authorized_keys"`

验证：`ssh open-voip echo ok`

## 本机一键发布（Python + paramiko）

1. 安装 Python 依赖：`pip install paramiko`
2. 已配置 SSH 公钥时可省略 `DEPLOY_SSH_PASSWORD`：

```powershell
cd C:\Users\yaojie\Desktop\open-voip
python scripts/deploy/push.py
```

3. 浏览器访问：`http://8.153.98.134/`（员工入口）、`http://8.153.98.134/guest/`（访客）

仅同步源码、稍后在服务器上手动编译：

```powershell
python scripts/deploy/push.py --sync-only
ssh root@8.153.98.134 "bash /opt/open-voip/src/scripts/deploy/remote-build.sh"
```

## 服务器上调试

```bash
# 登录
ssh root@8.153.98.134

# 看服务状态与日志
systemctl status open-switch open-call nginx
journalctl -u open-call -f
journalctl -u open-switch -f
tail -f /opt/open-call/logs/open-call/*.log
tail -f /opt/open-switch/logs/open-switch/*.log

# 在已同步的源码里改 Go 后快速重编（不经过本机）
export PATH=/usr/local/go/bin:$PATH
cd /opt/open-voip/src/open-call && go build -o /opt/open-call/open-call ./cmd/open-call
chown openvoip:openvoip /opt/open-call/open-call
systemctl restart open-call

# 前端
source /root/.nvm/nvm.sh
cd /opt/open-voip/src/open-call-web && npm run build
rsync -a --delete dist/ /var/www/open-call-web/
```

健康检查：

```bash
curl -s http://127.0.0.1:8082/health
curl -s http://127.0.0.1:8080/health/ready
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1/
```

## 安全建议

- **不要在聊天或 Git 中保存 root 密码**；部署完成后请修改 root 密码，并配置 SSH 公钥登录。
- 生产环境请关闭 `bootstrap`、更换 JWT 密钥，并将 `security.allowed_origins` 限制为真实域名。
- open-switch 的 Switch API 与 SIP 端口应对公网防火墙做最小放行（当前示例为内网 CC + 公网 SIP 场景）。

## 首次安装参考

若服务器尚未安装 PostgreSQL / systemd 单元，可参考现有单元：

- `/etc/systemd/system/open-switch.service`
- `/etc/systemd/system/open-call.service`

配置模板见各子项目 `deploy/config.example.yml`（复制为 `config.yml` 后按环境修改 DSN 与 SIP）。

呼叫失败与 WebSocket 事件契约见 [call-events.md](call-events.md)。

## HTTPS 与证书自动续期（open-switch.cn）

生产环境在 ECS 上使用 **Nginx 终结 TLS**；证书由 **acme.sh** 管理，文件路径：

- `/etc/nginx/ssl/open-switch.cn.crt`
- `/etc/nginx/ssl/open-switch.cn.key`

### 为何不用 certbot HTTP 验证

Let's Encrypt 从**境外**访问 ECS 的 80 端口常会超时（国内访问正常）。因此不要用 `certbot --nginx` 做 HTTP-01。应使用 **DNS-01**，并由 **阿里云 DNS API** 自动添加/删除 `_acme-challenge` TXT 记录。

### 一次性配置自动续期

1. **阿里云 RAM**：创建子账号 AccessKey，授予 `open-switch.cn` 的 DNS 解析权限（例如系统策略 `AliyunDNSFullAccess`，或仅 `AddDomainRecord` / `DeleteDomainRecord` / `DescribeDomainRecords` / `UpdateDomainRecord`）。
2. **在 ECS 上**创建密钥文件（勿写入 Git、勿发到聊天）：

   ```bash
   install -m 600 /dev/null /root/.acme.sh/ali.env
   # 编辑填入 Ali_Key / Ali_Secret
   ```

3. **切换为 API 签发**（会覆盖原先「手动 TXT」方式，之后 cron 可全自动续期）：

   ```bash
   bash /opt/open-voip/src/scripts/deploy/acme-dns-ali-setup.sh
   ```

   脚本见 [scripts/deploy/acme-dns-ali-setup.sh](../scripts/deploy/acme-dns-ali-setup.sh)。

4. **确认 cron**（acme.sh 安装时通常已写入）：

   ```bash
   crontab -l | grep acme
   ```

5. **试跑续期**（不真换证时可只看日志；强制试续期加 `--force`）：

   ```bash
   /root/.acme.sh/acme.sh --cron --home /root/.acme.sh
   ```

续期成功后会执行 `--install-cert` 时配置的 `nginx -t && systemctl reload nginx`。

### 手动 TXT 方式（不推荐长期使用）

若仍使用 `acme.sh --issue ... --dns --yes-I-know-dns-manual-mode-enough-go-ahead-please`，每次续期都要在阿里云再添加 TXT；**cron 无法无人值守续期**。请尽快改为上面的 `dns_ali` 方式。

### 访问与验收

- 员工：`https://open-switch.cn/`
- 访客：`https://open-switch.cn/guest/`
- `curl -sS https://open-switch.cn/health/ready`

**注意**：在 ECS **本机** `curl https://open-switch.cn/` 可能超时（回环访问公网 IP），以外网或本机浏览器为准。
