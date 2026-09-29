# §9 全量验收清单（骨架）

与《统一软交换架构实施方案》§9 对齐的自动化与手工项。完整 SIP/媒体压测仍依赖 `OPEN_VOIP_TEST_DSN` 与实网中继。

## 自动化（`go test`）

| 项 | 位置 |
|----|------|
| 接听 `Idempotency-Key` 重放 | `open-switch/internal/layers/control/acceptance_unified_test.go` |
| IVR / 排队 / 振铃 / **active** 崩溃恢复 | `recovery_test.go` |
| `os_commands` 幂等（需 DSN） | `open-switch/internal/store/commands_test.go` |
| 乐观锁 `VERSION_MISMATCH` | `version_guard_test.go` |

## 手工 / 集成（待环境）

- [ ] 双租户：两 integrator 各 register，Switch callback 与 open-call 投影隔离（对账可用 `GET /switch/v2/events`）
- [ ] SIP 中继入呼 → 队列 → 话机接听 → `leg.*` / `call.answered` 事件顺序
- [ ] 直控：`POST legs/sip` + `Idempotency-Key` 重复提交不双拨
- [ ] `PUT /calls/{id}/bridges/{bridgeId}` 替换腿后媒体可听
- [ ] `POST legs/{legId}/playbacks` 播放已上传 WAV 素材
- [ ] open-call：Switch callback → WebSocket；与 `GET /switch/v2/calls?status=open` 对账

## 运行

```bash
cd open-switch && go test ./...
# 带库集成：
OPEN_VOIP_TEST_DSN='postgres://...' go test ./internal/store -run TestCommandIdempotency
```
