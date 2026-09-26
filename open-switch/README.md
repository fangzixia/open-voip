# open-switch

软交换服务：WebRTC/SIP 媒体、Call FSM、对内 **Switch API**（`/switch/v1`）。

- 对接说明（含 open-call 范例）：[../docs/open-switch对接说明.md](../docs/open-switch对接说明.md)
- 配置示例：[deploy/config.example.yml](deploy/config.example.yml)

## 启动

```bash
go build -o bin/open-switch ./cmd/open-switch
go run ./cmd/open-switch -config deploy/config.example.yml
```

默认监听 `127.0.0.1:8082`（内网）。`integration.mode: call_center` 时配置 `integration.platform_base_url` 指向 open-call；`external` 时由其他可信应用调用 `/switch/v1/calls/direct` 和双腿桥接接口，无需 Platform API。Switch 信任持有服务密钥的调用方，不处理终端用户鉴权。完整流程见[对接说明](../docs/open-switch对接说明.md)。

视频录像需要 FFmpeg。设置 `recordings.ffmpeg_path` 为可执行文件路径，或将 FFmpeg 加入 `PATH`；
`recordings.video_format` 可选 `webm`（VP8/Opus）或 `mp4`（H.264/AAC）。每通视频呼叫保存一份同时含画面和声音的文件。
下载接口可通过 `format=webm|mp4` 临时转换为另一种格式，不会长期保存第二份成品。

## 测试

```bash
go test ./...
```
