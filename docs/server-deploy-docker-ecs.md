# ECS Docker 部署（open-switch + open-call）

在单台 ECS 上用 **Docker Compose** 运行软交换与呼叫中心后端；**PostgreSQL 仍在宿主机**，通过各服务 `config.yml` 的 `database.dsn` 连接。前端继续使用宿主机 **Nginx + `/var/www/open-call-web`**，不把数据库放进 Docker。

## 架构摘要

| 组件 | 运行方式 |
|------|----------|
| open-switch | 容器，`network_mode: host`，配置/录音绑定目录 |
| open-call | 容器，`network_mode: host` |
| PostgreSQL | 宿主机（常见 `127.0.0.1:5432`） |
| open-call-web | 宿主机静态文件 + Nginx |
| 镜像 | GHCR：`ghcr.io/<owner>/open-voip-open-switch`、`open-voip-open-call` |

HTTP 监听：**`0.0.0.0:8082`（Switch）**、**`0.0.0.0:8080`（Call）**。同机互调 URL 仍为 `http://127.0.0.1:...`（`switch_base_url`、`events_callback_url`）。

**安全组**：公网仅放行 80/443（及 SIP/RTP）；**勿对公网开放 8080、8082**。

## 前置条件

- Linux ECS（与 [server-deploy-quickstart.md](./server-deploy-quickstart.md) 相同：Postgres、库 `openvoip_switch` / `openvoip_call`、Nginx、TLS 可选）
- Docker Engine + Compose plugin
- 仓库源码或发布包在 **`/opt/open-voip/src`**（含根目录 `compose.yml`）
- GitHub Actions 已推送 GHCR 镜像（见 [.github/workflows/docker-publish.yml](../.github/workflows/docker-publish.yml)）

### 登录 GHCR（ECS 上）

使用具有 `read:packages` 的 GitHub PAT：

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u YOUR_GITHUB_USER --password-stdin
```

公开包可省略登录；私有包必须登录。

## 从 systemd 二进制迁移

1. **排空或接受重启**：open-switch 单实例，升级/切换会挂断在途通话。

2. **同步代码**（含 `compose.yml`、本目录脚本）到 `/opt/open-voip/src`。

3. **生成运行时配置**（复制现网 `config.yml`，改 listen 为 `0.0.0.0`，**不改 DSN**）：

   ```bash
   cd /opt/open-voip/src
   python3 deploy/docker/migrate-ecs-config.py \
     --runtime-dir /opt/open-voip/runtime \
     --env-file /opt/open-voip/.env \
     --switch-src /opt/open-switch/config.yml \
     --call-src /opt/open-call/config.yml \
     --force
   ```

   脚本会把 `/opt/open-switch`、`/opt/open-call` 下除 `config.yml` 外的数据（录音、日志等）复制到 `runtime/` 对应目录（若尚未存在）。

4. **确认 Postgres**：

   ```bash
   pg_isready -h 127.0.0.1 -p 5432 -U openvoip
   ```

5. **部署**：

   ```bash
   chmod +x deploy/docker/ecs-deploy.sh
   OPEN_VOIP_ROOT=/opt/open-voip/src ENV_FILE=/opt/open-voip/.env \
     deploy/docker/ecs-deploy.sh
   ```

6. **验收**：

   ```bash
   curl -sS http://127.0.0.1:8082/health
   curl -sS http://127.0.0.1:8080/health/ready
   curl -sS https://open-switch.cn/health/ready   # 或你的域名
   ```

   业务验收见 [call-events.md](./call-events.md) ECS 清单、[outbound-pstn-ecs.md](./outbound-pstn-ecs.md)。

7. Nginx 仍反代 `http://127.0.0.1:8080`（与 Call 监听 `0.0.0.0:8080` 兼容）。前端构建流程不变。

## 升级镜像

编辑 `/opt/open-voip/.env` 中的 `IMAGE_TAG`（例如 `main` 或 commit SHA），然后：

```bash
cd /opt/open-voip/src
docker compose --env-file /opt/open-voip/.env pull open-switch open-call
docker compose --env-file /opt/open-voip/.env up -d open-switch open-call
```

或再次执行 `ecs-deploy.sh`。

## 本地开发（可选）

- **仅后端容器**（ECS 同款）：配置好 `runtime/` 与 `.env` 后 `docker compose up -d`（默认**不**启动 `web`）。
- **含前端容器**：`docker compose --profile with-web up -d`。
- **全新本地环境 + 容器 Postgres**（非 ECS）：`python3 deploy/docker/init.py` 后 `docker compose -f compose.yml -f compose.database.yml up -d`。ECS **不要**使用 `compose.database.yml`。

## 回滚到 systemd

1. `docker compose --env-file /opt/open-voip/.env down`
2. 从 `/opt/open-voip/backups/<timestamp>/` 恢复 config（如有需要）
3. `systemctl enable --now open-switch open-call`

## 环境变量（`.env`）

参见仓库根目录 [.env.example](../.env.example)。ECS 典型项：

- `GHCR_REGISTRY=ghcr.io/fangzixia`
- `IMAGE_TAG=main`
- `SWITCH_STATE_DIR` / `CALL_STATE_DIR` → 含 `config.yml` 与录音的绑定目录

## 常见问题

| 现象 | 处理 |
|------|------|
| 8080/8082 端口占用 | 确认 systemd 单元已 stop/disable；`ss -lntp` |
| 容器内连不上 DB | DSN 仍为宿主机地址；检查 Postgres `listen_addresses` |
| 录音不可写 | `runtime/open-switch` 属主 `10001:10001` |
| pull 403 | `docker login ghcr.io`；检查包可见性 |
