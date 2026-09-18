# Open VoIP — 应用代码

本目录包含服务端、前端与部署配置；文档见仓库根目录 [../docs/](../docs/)。

## 依赖

- Go 1.26+
- Node.js 20+
- PostgreSQL 16+

## 数据库（开发）

在仓库根目录执行：

```bash
docker compose -f app/deploy/docker-compose.dev.yml up -d
```

`app/deploy/config.example.yml` 中 DSN 与上述 compose 对齐。

## 后端

在本目录（`app/`）下：

```bash
go mod download
go build -o bin/open-voip.exe ./cmd/open-voip
go run ./cmd/open-voip -config deploy/config.example.yml
```

探针：`GET http://127.0.0.1:8080/health` → `OK`

## 前端

```bash
cd frontend
npm install
npm run build    # 产出到 ../static/{agent,guest,admin}
npm run dev      # /api 代理到 8080
```

构建后重启后端，访问 http://127.0.0.1:8080/agent/ 等。

## 前端独立部署（与 API 分机）

**API（open-voip）**

```bash
go run ./cmd/open-voip -config deploy/config.example.split.yml
```

- `static_serve: false`，不托管 UI
- `cors.allowed_origins` 必须包含 UI 的 Origin（如 `https://ui.cc.internal`）
- `server.public_url` 为 **API** 地址（WebRTC/信令），可与 UI 域名不同

**UI（Nginx / 静态托管）**

```bash
cd frontend
cp .env.example .env   # 设置 VITE_API_BASE=https://api.cc.internal
npm run build:split    # 产出到 frontend/dist/
```

将 `dist/` 部署到 Web 服务器，参考 `deploy/nginx-frontend.example.conf`。

**本地联调（UI 5173 + API 8080）**

1. API 使用 `config.example.split.yml`，并在 `cors.allowed_origins` 中加入 `http://127.0.0.1:5173`
2. `frontend/.env` 设置 `VITE_API_BASE=http://127.0.0.1:8080`
3. `npm run dev`（不再走 Vite 代理，直连 API）

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
| `frontend/` | Lit 三端 + `shared/` |
| `deploy/` | config 示例、systemd、开发 compose |
| `static/` | 前端构建输出（gitignore） |
