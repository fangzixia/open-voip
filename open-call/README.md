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

升级与端到端验收步骤见 [安装说明](deploy/INSTALL.md) 和 [SIP 生产验收](../docs/sip-production-acceptance.md)。

## 测试

```bash
go test ./...
```
