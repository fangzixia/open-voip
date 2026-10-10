# 内网 Web 呼叫中心。

```
open-voip/
  docs/           # 需求、架构、API；含 open-switch 对接说明、服务器部署速查
  open-switch/    # 软交换（媒体 + 呼叫控制 + Switch API）
  open-call/      # 呼叫中心（业务 + BFF + Switch 客户端）
  open-call-web/  # Lit 坐席 / 访客 / 管理端
  .github/        # CI
```

- 软交换：[open-switch/README.md](open-switch/README.md)
- 呼叫中心：[open-call/README.md](open-call/README.md)
- VoIP 流程与技术方案总览：[docs/voip-architecture-flows.md](docs/voip-architecture-flows.md)
- 与 open-switch 对接：[docs/open-switch对接说明.md](docs/open-switch对接说明.md)、[docs/switch-standalone-runbook.md](docs/switch-standalone-runbook.md)
- 前端：[open-call-web/README.md](open-call-web/README.md)

- 双服务边界、SIP 中继与坐席配置、上线验收：[SIP 上线验收](docs/sip-production-acceptance.md)。
- 服务器一键发布与 SSH 调试：[server-deploy-quickstart.md](docs/server-deploy-quickstart.md)。
