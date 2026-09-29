# 文档

设计材料与对接说明。**运行时集成以 Switch register + HTTP 事件 callback 为准**（见下方「集成」）。

| 文档 | 说明 |
|------|------|
| [open-switch对接说明.md](./open-switch对接说明.md) | Switch API、登记、鉴权、事件 callback |
| [switch-standalone-runbook.md](./switch-standalone-runbook.md) | Switch 独立部署与 register 步骤 |
| [requirements.md](./requirements.md) | 功能需求与验收 ID |
| [architecture.md](./architecture.md) | 四级分层与 Port |
| [统一软交换架构实施方案.md](./统一软交换架构实施方案.md) | 架构决策与迁移（部分段落为历史差距表） |
| [section9-acceptance-checklist.md](./section9-acceptance-checklist.md) | §9 验收清单 |
| [sip-production-acceptance.md](./sip-production-acceptance.md) | SIP/上线验收 |
| [api/](./api/) | OpenAPI（open-call REST） |
| [events.md](./events.md) | WebSocket 事件 |
| [权限登录系统对接说明.md](./权限登录系统对接说明.md) | 登录、OIDC 与 RBAC |

应用代码见 [open-call/](../open-call/)、[open-switch/](../open-switch/)、[open-call-web/](../open-call-web/)。
