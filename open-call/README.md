# open-call

呼叫中心服务：坐席/队列/ACD、CDR、对浏览器 **REST/WS**，并通过 BFF 代理通话信令到 open-switch。

- 对接说明：[../docs/open-switch对接说明.md](../docs/open-switch对接说明.md)
- 浏览器 API 契约：[../docs/api/openapi.yaml](../docs/api/openapi.yaml)
- 配置示例：[deploy/config.example.yml](deploy/config.example.yml)

## 启动

```bash
go build -o bin/open-call ./cmd/open-call
go run ./cmd/open-call -config deploy/config.example.yml
```

默认监听 `0.0.0.0:8080`；前端 `VITE_API_BASE` 指向本服务。

生产模式将 SIP 注册、设备凭证、RTP、ICE/TURN 和录音文件全部交给 open-switch。open-call 只保存业务配置与元数据，并提供持久 Webhook 队列、可撤销登录会话、一次性访客令牌、配置预检/恢复、审计、报表和运维探针。生产配置必须关闭 bootstrap，使用强 JWT/集成密钥，并设置浏览器 Origin 白名单。

## 权限与单点登录

升级会把现有 admin、supervisor、agent 用户迁移为内置角色，并保留用户和坐席 ID。管理员可在管理端“用户与权限”维护自定义角色、功能操作权限、用户角色和外部组映射。坐席是否可签入取决于其坐席资料；OIDC 自动创建的用户须由管理员补齐分机及终端信息。

启用 `oidc` 前，先配置身份平台的 issuer、client ID/secret、回调地址、`groups_claim` 和加密密钥，并确认身份平台在令牌刷新时提供最新组声明。刷新时会同步外部组对应的角色；若无法获取最新组信息或已无映射角色，当前会话失效，用户需重新登录。组未映射角色时拒绝登录；已有同名或同邮箱账号需管理员通过稳定的 `issuer + sub` 显式绑定。仅 `emergency_admin` 保留密码登录，且该用户必须启用并拥有内置 admin 角色。身份平台离线时服务仍可启动，应急管理员可使用密码入口。退出仅撤销本项目会话。

升级与端到端验收步骤见 [安装说明](deploy/INSTALL.md) 和 [SIP 生产验收](../docs/sip-production-acceptance.md)。

## 测试

```bash
go test ./...
```
