# open-call-web（Lit + Vite）

坐席 / 访客 / 管理三端静态 UI，对接 **open-call** 的 `/api/v1`（通话经 BFF 代理至 open-switch）。

## 开发

另开终端启动 open-call（`-config deploy/config.example.yml`）。API 对任意浏览器 Origin 开放 CORS。

```bash
cd open-call-web
npm install
npm run dev    # http://127.0.0.1:5173/agent/ 、 /guest/ 、 /admin/
```

未设置 `VITE_API_BASE` 时，开发服务器把 `/api`、`/health` 代理到 `http://127.0.0.1:8080`。

## 生产构建

```bash
cp .env.example .env   # 设置 VITE_API_BASE 为 API 根地址
npm run build          # 产出到 dist/
```

将 `dist/` 交给 Nginx，示例：[open-call/deploy/nginx-frontend.example.conf](../open-call/deploy/nginx-frontend.example.conf)。
