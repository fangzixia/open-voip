# open-switch

软交换服务：WebRTC/SIP 媒体、Call FSM、对内 **Switch API**（`/switch/v1`）。

- 对接说明（含 open-call 范例）：[../docs/open-switch对接说明.md](../docs/open-switch对接说明.md)
- 配置示例：[deploy/config.example.yml](deploy/config.example.yml)

## 启动

```bash
go build -o bin/open-switch ./cmd/open-switch
go run ./cmd/open-switch -config deploy/config.example.yml
```

默认监听 `127.0.0.1:8082`（内网）；需配置 `integration.platform_base_url` 指向 open-call。

## 测试

```bash
go test ./...
```
