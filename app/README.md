# Open VoIP — 服务端

本目录为 Go API 进程（REST / WebSocket / 媒体信令）。前端在仓库根目录 [../frontend/](../frontend/)。设计文档见 [../docs/](../docs/)。

## 依赖

- Go 1.27.1+
- PostgreSQL 16+

## 数据库（开发）

本机安装 PostgreSQL 16+，建库并与 `deploy/config.example.yml` 中 DSN 对齐，例如：

```sql
CREATE USER openvoip WITH PASSWORD 'change_me';
CREATE DATABASE openvoip OWNER openvoip;
```

DSN 默认：`host=127.0.0.1 user=openvoip password=change_me dbname=openvoip port=5432 sslmode=disable`。

## 启动

```bash
go mod download
go build -o bin/open-voip.exe ./cmd/open-voip
go run ./cmd/open-voip -config deploy/config.example.yml
```

探针：`GET http://127.0.0.1:8080/health` → `OK`

空库首次启动写入演示账号（`bootstrap`，默认密码 `changeme`）：

- `admin` / `changeme`
- `agent1` / `changeme`（分机 1001）

本进程**不托管网页**。请另开终端按 [../frontend/README.md](../frontend/README.md) 启动 `npm run dev`（默认代理 `/api` 到 8080）。API 对任意浏览器 Origin 开放 CORS。

生产环境将前端 `dist/` 交给 Nginx。API 与 UI 根地址由部署方各自配置（前端 `VITE_API_BASE`、反代域名等）。安装与证书见 [deploy/INSTALL.md](deploy/INSTALL.md)。

## 测试

```bash
go test ./...
# 迁移集成测试（需 Postgres）：
# set OPEN_VOIP_TEST_DSN=...
go test ./internal/store/...
```

## 目录

| 路径 | 说明 |
|------|------|
| `cmd/open-voip` | 进程入口 |
| `internal/` | 分层业务与 `app` 适配层 |
| `deploy/` | config 示例、systemd、Nginx、安装说明 |
