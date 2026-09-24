# Open VoIP

内网 Web 呼叫中心。

```
open-voip/
  docs/           # 需求、架构、API；含 open-switch 对接说明
  open-switch/    # 软交换（媒体 + 呼叫控制 + Switch API）
  open-call/      # 呼叫中心（业务 + BFF + Platform API）
  open-call-web/  # Lit 坐席 / 访客 / 管理端
  .github/        # CI
```

- 软交换：[open-switch/README.md](open-switch/README.md)
- 呼叫中心：[open-call/README.md](open-call/README.md)
- 与 open-switch 对接（唯一正文）：[docs/open-switch对接说明.md](docs/open-switch对接说明.md)
- 前端：[open-call-web/README.md](open-call-web/README.md)

- 双服务边界、SIP 中继与坐席配置、上线验收：[SIP 上线验收](docs/sip-production-acceptance.md)。
