# open-call-web（Lit + Vite）

员工使用根路径 `/` 的单页入口，按 `/auth/me` 权限显示管理功能和坐席工作台；访客仍使用 `/guest/`。页面对接 **open-call** 的 `/api/v1`（通话经 BFF 代理至 open-switch）。

启用后端 OIDC 后，统一登录页显示“统一身份平台登录”。回调将一次性票据放在根路径 URL 片段，页面立即清除片段并向后端交换本项目令牌；保留应急管理员密码入口。

## 前端结构

- `admin/admin-app.js`、`agent/agent-app.js`、`guest/guest-app.js` 管理各端状态、接口调用和事件处理。
- 各端的 `views/` 按页面或功能放置 Lit 模板。`views/shell.js` 负责导航和页面选择，只把页面需要的状态与回调传给具体视图。
- `admin/controllers/` 处理用户与权限操作及 IVR 接口适配；用户和坐席账号统一在“用户与权限”创建，“坐席”页负责坐席列表、签出和报表。
- `guest/components/service-card.js` 统一语音和视频服务卡片，`agent/components/dial-controls.js` 统一两处外呼输入。
- `shared/components/ivr/` 包含 IVR 编辑器、流程模型与对应视图；管理端通过自定义元素使用编辑器。
- `shared/components/` 提供三端共用的导航壳、登录输入、反馈、拨号盘、面板、表单字段与复选框、数据表格、状态标签和描述列表；各页面只提供数据与事件回调。`shared/display.js` 统一坐席状态文案与颜色，`shared/styles/` 按基础、布局、控件和通话场景组织样式，由 `shared/styles/index.js` 统一组合；接口和媒体模块也由三端复用。

## 开发

另开终端启动 open-call（`-config deploy/config.example.yml`）。API 对任意浏览器 Origin 开放 CORS。

```bash
cd open-call-web
npm install
npm run dev    # http://127.0.0.1:5173/ 、 /guest/
```

未设置 `VITE_API_BASE` 时，开发服务器把 `/api`、`/health` 代理到 `http://127.0.0.1:8080`。

## 生产构建

```bash
cp .env.example .env   # 设置 VITE_API_BASE 为 API 根地址
npm run build          # 产出到 dist/
```

将 `dist/` 交给 Nginx，示例：[open-call/deploy/nginx-frontend.example.conf](../open-call/deploy/nginx-frontend.example.conf)。
