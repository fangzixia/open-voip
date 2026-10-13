# ECS Docker 部署（open-switch + open-call）

在单台 ECS 上用 **`docker run`** 运行软交换与呼叫中心（**两个独立容器、两个独立升级脚本**）。**PostgreSQL 为独立服务**，本仓库部署脚本不负责建库或起库，仅在各服务 `config.yml` 的 `database.dsn` 中连接现网库。前端已内置于 **open-call 镜像**（容器内 Nginx，默认 **`127.0.0.1:8088`**）；宿主机 Nginx 反代该端口。

## 架构摘要

| 组件 | 运行方式 |
|------|----------|
| open-switch | 容器 `--network host`，配置/录音绑定目录 |
| open-call | 容器 `--network host`；API **8080**，内嵌 Web **8088** |
| PostgreSQL | 独立服务（DSN 由运维写入 `config.yml`） |
| 边缘 Nginx | 宿主机 80/443 → `http://127.0.0.1:8088` |
| 镜像 | GHCR：`ghcr.io/<owner>/open-voip-open-switch`、`open-voip-open-call` |

HTTP 监听：**`0.0.0.0:8082`（Switch）**、**`0.0.0.0:8080`（Call API）**、**`0.0.0.0:8088`（Call Web）**。同机互调 URL 仍为 `http://127.0.0.1:...`（`switch_base_url`、`events_callback_url`）。

**安全组**：公网仅放行 80/443（及 SIP/RTP）；**勿对公网开放 8080、8082、8088**。

## 前置条件

- Linux ECS（与 [server-deploy-quickstart.md](./server-deploy-quickstart.md) 相同：已就绪的 PostgreSQL、`openvoip_switch` / `openvoip_call` 库、Nginx、TLS 可选）
- **Docker Engine**（建议 >= 20.10；[`deploy/docker/deploy.sh`](../deploy/docker/deploy.sh) 会检测是否已安装且可连接）
- 仓库源码或发布包在 **`/opt/open-voip/src`**
- GitHub Actions 已推送 GHCR 镜像（见 [.github/workflows/docker-publish.yml](../.github/workflows/docker-publish.yml)）

### 登录 GHCR（ECS 上）

使用具有 `read:packages` 的 GitHub PAT：

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u YOUR_GITHUB_USER --password-stdin
```

公开包可省略登录；私有包必须登录。

## 从 systemd 二进制迁移

1. **排空或接受重启**：open-switch 单实例，升级/切换会挂断在途通话。

2. **同步代码**（含 `deploy/docker/deploy.sh`）到 `/opt/open-voip/src`。

3. **准备运行时目录与配置**（手工；**不改 `database.dsn`**）：

   ```bash
   install -d -m 750 /opt/open-voip/runtime/open-switch /opt/open-voip/runtime/open-call
   cp /opt/open-switch/config.yml /opt/open-voip/runtime/open-switch/
   cp /opt/open-call/config.yml /opt/open-voip/runtime/open-call/
   # 可选：复制录音/日志等（勿覆盖已存在的 config.yml）
   rsync -a --exclude=config.yml /opt/open-switch/ /opt/open-voip/runtime/open-switch/
   rsync -a --exclude=config.yml /opt/open-call/ /opt/open-voip/runtime/open-call/
   chown -R 10001:10001 /opt/open-voip/runtime/open-switch /opt/open-voip/runtime/open-call
   chmod 600 /opt/open-voip/runtime/open-switch/config.yml /opt/open-voip/runtime/open-call/config.yml
   ```

   编辑两份 `config.yml`：Switch `server.listen` → `"0.0.0.0:8082"`；Call `listen` → `0.0.0.0:8080`；同机 URL 使用 `http://127.0.0.1:...`（`switch_base_url`、`events_callback_url`）。

4. **`.env`**：自仓库复制 [.env.example](../.env.example) 到 `/opt/open-voip/.env`，设置 `SWITCH_STATE_DIR`、`CALL_STATE_DIR`、`IMAGE_TAG` 等。

5. **部署**（[`deploy/docker/deploy.sh`](../deploy/docker/deploy.sh)）：

   ```bash
   cd /opt/open-voip/src
   chmod +x deploy/docker/deploy.sh
   ENV_FILE=/opt/open-voip/.env deploy/docker/deploy.sh ecs all
   ```

   仅升级一侧：

   ```bash
   ENV_FILE=/opt/open-voip/.env deploy/docker/deploy.sh ecs switch
   ENV_FILE=/opt/open-voip/.env deploy/docker/deploy.sh ecs call
   ```

6. **宿主机 Nginx**：使用 [deploy/nginx/open-voip.conf](../deploy/nginx/open-voip.conf)（反代 **8088**），`nginx -t && systemctl reload nginx`。

7. **验收**：

   ```bash
   curl -sS http://127.0.0.1:8082/health
   curl -sS http://127.0.0.1:8080/health/ready
   curl -sS http://127.0.0.1:8088/container-health
   curl -sS -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8088/
   curl -sS https://open-switch.cn/health/ready   # 或你的域名
   ```

## 升级镜像

编辑 `/opt/open-voip/.env` 中的 `IMAGE_TAG`，然后执行 `deploy/docker/deploy.sh ecs switch|call` 或 `deploy.sh run switch|call`（内部 `docker pull` + 重建容器）。

可选：仅挂载宿主机二进制热修（不含前端层）：

```bash
export SWITCH_BINARY_MOUNT=/opt/open-switch/open-switch
export CALL_BINARY_MOUNT=/opt/open-call/open-call
# 再运行 deploy.sh run switch|call
```

## 本地开发（可选）

1. 创建 `runtime/open-switch`、`runtime/open-call`，放入 `config.yml`（可参考各项目 `deploy/config.example.yml`），并配置 `database.dsn`。
2. 复制 `.env.example` 为 `.env`，调整路径与 `IMAGE_TAG=local`。
3. 启动：

   ```bash
   source .env
   PULL=0 deploy/docker/deploy.sh run switch
   PULL=0 deploy/docker/deploy.sh run call
   ```

本地构建：

```bash
docker build -f open-switch/Dockerfile -t ghcr.io/fangzixia/open-voip-open-switch:local open-switch
docker build -f open-call/Dockerfile -t ghcr.io/fangzixia/open-voip-open-call:local .
```

## 回滚到 systemd

1. `docker rm -f open-switch open-call`
2. 从 `/opt/open-voip/backups/<timestamp>/` 恢复 config（如有需要）
3. `systemctl enable --now open-switch open-call`

## 环境变量（`.env`）

参见 [.env.example](../.env.example)。ECS 典型项：

- `GHCR_REGISTRY`、`IMAGE_TAG`
- `SWITCH_STATE_DIR` / `CALL_STATE_DIR`
- `WEB_LISTEN`、`CALL_HTTP_UPSTREAM`、`WEB_HEALTH_URL`

## 常见问题

| 现象 | 处理 |
|------|------|
| 8080/8082/8088 端口占用 | 确认 systemd 单元已 stop/disable；`ss -lntp` |
| 容器内连不上 DB | 检查 `config.yml` 的 `database.dsn` 与网络/防火墙（本仓库脚本不管理数据库） |
| 录音不可写 | `runtime/open-switch` 属主 `10001:10001` |
| pull 403 | `docker login ghcr.io`；检查包可见性 |
| 页面 502 | 边缘 Nginx 是否指向 **8088**；`curl 127.0.0.1:8088/container-health` |
