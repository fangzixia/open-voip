# 分层实施规范

> 概念权威：[architecture.md §2](./architecture.md)  
> 技术选型：[technical-design.md §2](./technical-design.md)

本文说明 **Go 包边界**、Port 路径、禁止耦合与 CI depguard 规则。

---

## 1. 与 architecture 的关系

- **architecture.md**：对外说明四层职责、DAG、Port 名称  
- **本文**：实现路径、depguard、PR 检查项  

---

## 2. Port 一览（含包路径）

| Port | 定义 | 实现（示例路径） | 调用方 |
|------|------|------------------|--------|
| CallControlPort | `internal/ports/call_control.go` | `internal/layers/control/service.go` | L4 guest/outbound；app/http |
| SignalingPort | `internal/ports/signaling.go` | `internal/layers/control/service.go` | **仅 app/http/media**（禁止直连 MediaPort） |
| CallPersistencePort | `internal/ports/call_persist.go` | `internal/store/call_persist.go` | **仅 L3**（L3 禁止 import store） |
| MediaPort | `internal/ports/media.go` | `internal/layers/media/service.go` | **仅 L3** |
| ACDDispatchPort | `internal/ports/acd.go` | `internal/layers/biz/queue/service.go` | L3 |
| ConfigSnapshotPort | `internal/ports/config_snapshot.go` | `internal/layers/biz/configpub` | L3 ivr/runtime |
| AgentDirectoryPort | `internal/ports/agent_directory.go` | `internal/layers/biz/agent` | L3（含 SetState 乐观锁） |
| RecordingPolicyPort | `internal/ports/recording_policy.go` | `internal/layers/biz/queue` | L3 |
| RecordingStorePort | `internal/ports/recording_store.go` | `internal/layers/biz/recmeta` | L3 |
| CDRRecorderPort | `internal/ports/cdr.go` | `internal/layers/biz/cdr` | L3 |
| CallEventPublisher | `internal/ports/events.go` | `internal/app/ws` | L3 注入 |
| AgentEventPublisher | `internal/ports/events.go` | `internal/app/ws` | L4 注入 |
| WebhookDispatcher | `internal/ports/webhook.go` | `internal/layers/biz/webhook` | `app/ws` / 管理 API |

跨层 DTO 仅放在 `internal/ports` 或 `internal/ports/dto`，禁止 L3 引用 L4 的 GORM model。

---

## 3. 禁止耦合清单

| 反模式 | 正确做法 |
|--------|----------|
| `layers/biz/queue` import `layers/control/fsm` | L4 实现 `ACDDispatchPort` |
| 签入时 `CreateRoom` | Room 在 L3 Answer 后 |
| L2 写 CDR | L3 调 `CDRRecorderPort` |
| L2 读 IVR/队列 DB | L3 读 `ConfigSnapshotPort` |
| handler 内 ACD 算法 | 仅在 L4 `queue` 包 |
| L4 import `pion/*` | L3 调 `MediaPort` |
| L4/L3 混在一个 `events` 包发 WS | `ports` 定义类型；L3/L4 分 publisher |

---

## 4. 组合根规则

- **允许** import 各层 concrete：`app/cmd/open-voip`、`app/internal/app`（组合根）  
- **禁止**：`internal/app/wire` 作为 DI 框架；各 `layers/*` 互相 import 实现包  
- `internal/app/http`、`internal/app/ws` 只依赖 **接口**（Service / Port）  

启动顺序见 [technical-design.md §2.11](./technical-design.md)。

---

## 5. depguard（`app/.golangci.yml`）

在 golangci-lint 中启用 depguard，规则要点：

| 包前缀 | 禁止 import |
|--------|-------------|
| `internal/layers/biz` | `internal/layers/control`, `internal/layers/media`, `github.com/pion/*` |
| `internal/layers/control` | `internal/layers/biz`, `internal/store`（SQL 仅经 Port 回调 L4） |
| `internal/layers/media` | `internal/layers/biz`, `internal/layers/control`, `internal/store` |

L2 允许 `github.com/pion/*`、`github.com/emiago/sipgo`、`github.com/icholy/digest`（见 `app/.golangci.yml`）。
| `internal/layers/*` | 互相之间除 `ports` 外禁止 |

`cmd/open-voip` 与 `internal/app`（组合根 `run.go`）不受「跨层 import」禁止（白名单）。

---

## 6. PR 评审检查项

- [ ] 新 package 所属层是否正确  
- [ ] 跨层是否仅通过 `ports`  
- [ ] 导出 struct/Port 是否有中文 doc（architecture §5）  
- [ ] GORM model 字段是否有 `comment` tag  

---

## 修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| v0.2 | 2026-09-18 | 补充 RecordingStorePort、WebhookDispatcher |
