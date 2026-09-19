# Open VoIP — 前端（Lit + Vite）

与 Go 服务端目录分离：本目录只包含坐席 / 访客 / 管理三端。API 由 `app/` 中的 `open-voip` 提供。

## 开发

另开终端启动后端（`app/` 下 `-config deploy/config.example.yml`）。API 对任意浏览器 Origin 开放 CORS。

```bash
cd frontend
npm install
npm run dev    # http://127.0.0.1:5173/agent/ 、 /guest/ 、 /admin/
```

未设置 `VITE_API_BASE` 时，开发服务器把 `/api`、`/health` 代理到 `http://127.0.0.1:8080`。

## 生产构建

```bash
cp .env.example .env   # 设置 VITE_API_BASE 为 API 根地址
npm run build          # 产出到 dist/
```

将 `dist/` 交给 Nginx，示例：[app/deploy/nginx-frontend.example.conf](../app/deploy/nginx-frontend.example.conf)。
