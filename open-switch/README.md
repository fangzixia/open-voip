# open-switch

软交换服务：WebRTC/SIP 媒体、Call FSM、对内 **Switch API**（`/switch/v1`），以及在 Switch 库内运行的 DID/队列/ACD/坐席话务状态与 IVR。

- 对接说明（含 open-call 范例）：[../docs/open-switch对接说明.md](../docs/open-switch对接说明.md)
- 配置示例：[deploy/config.example.yml](deploy/config.example.yml)

## 启动

```bash
go build -o bin/open-switch ./cmd/open-switch
go run ./cmd/open-switch -config deploy/config.example.yml
```

默认监听 `127.0.0.1:8082`（内网）。在 YAML 配置 `integration.events_callback_url`（指向 CC 事件入口，如 `http://127.0.0.1:8080/api/v1/integration/switch/events`）；事件落库后 POST 到该 URL。CC↔Switch **无鉴权**，依赖私有网络。完整流程见[对接说明](../docs/open-switch对接说明.md)与 [runbook](../docs/switch-standalone-runbook.md)。

**部署约束：单实例。** 通话 FSM、媒体房间、SIP registrar 与事件投递均在进程内存；多进程共享同一数据库会分裂状态，不支持双活。滚动升级请先排空通话或接受重启挂断。

视频录像需要 FFmpeg。设置 `recordings.ffmpeg_path` 为可执行文件路径，或将 FFmpeg 加入 `PATH`；
`recordings.video_format` 可选 `webm`（VP8/Opus）或 `mp4`（H.264/AAC）。每通视频呼叫保存一份同时含画面和声音的文件。
下载接口可通过 `format=webm|mp4` 临时转换为另一种格式，不会长期保存第二份成品。

## 测试

```bash
go test ./...
```
