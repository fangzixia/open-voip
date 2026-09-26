# 文档

本目录仅存放 **需求、架构、API 契约** 等设计材料，不含可运行代码。

| 文档 | 说明 |
|------|------|
| [requirements.md](./requirements.md) | 功能需求与验收 ID |
| [architecture.md](./architecture.md) | 四级分层与 Port |
| [technical-design.md](./technical-design.md) | 实现级设计 |
| [implementation-plan.md](./implementation-plan.md) | 实施计划 |
| [api/](./api/) | OpenAPI（REST 对接） |
| [权限登录系统对接说明.md](./权限登录系统对接说明.md) | 登录、OIDC 与 RBAC 对接流程 |
| [events.md](./events.md) | WebSocket 事件 |

应用代码见 **[../open-call/](../open-call/)**、**[../open-switch/](../open-switch/)** 与 **[../open-call-web/](../open-call-web/)**（前端）。

- 双服务边界、SIP 中继与坐席配置、上线验收：[SIP 上线验收](sip-production-acceptance.md)。
