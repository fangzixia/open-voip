# open-switch

软交换服务：WebRTC/SIP 媒体、Call FSM、对内 **Switch API**（`/switch/v1`），以及在 Switch 库内运行的 DID/队列/ACD/坐席话务状态与 IVR。

Switch 提供通用 IVR 接续与原始按键采集，业务系统负责评价等业务语义。`POST /calls/{id}/ivr` 显式指定流程；`collect_input` 发布 `ivr.input_collected`。队列技术能力统一在 Switch 校验。坐席已有振铃、通话或保持中的呼叫时，新外呼返回 `409 AGENT_BUSY`，不会结束已有通话。旧评价接口、节点及事件已移除，升级须按[对接说明](../docs/open-switch对接说明.md)转换并发布全部流程。

- 对接说明（含 open-call 范例）：[../docs/open-switch对接说明.md](../docs/open-switch对接说明.md)
- 配置示例：[deploy/config.example.yml](deploy/config.example.yml)

## 启动

完整构建和运行使用 Linux/WSL、Go 1.27.1 与 CGO。Ubuntu 24.04 安装原生音频依赖：

```bash
sudo apt-get update
sudo apt-get install -y build-essential pkg-config libsoxr-dev libspandsp-dev
export CGO_ENABLED=1
```

本期 G.711 链路不需要 libopus。libsoxr 和 SpanDSP 缺失时构建失败，不提供另一套音频引擎或纯 Go 降级。原生 Windows 不作为完整构建、运行与验收环境。

```bash
go build -o bin/open-switch ./cmd/open-switch
go run ./cmd/open-switch -config deploy/config.example.yml
```

可用 `bash tools/build-linux.sh` 生成带依赖记录和许可证的 Linux 构建目录。目录包含二进制校验和、完整 Go 模块版本、原生库版本与动态链接信息、各模块许可证和受控 SDK 修改清单。运行实例同样需要系统提供 libsoxr/SpanDSP 动态库。

默认监听 `127.0.0.1:8082`（内网）。在 YAML 配置 `integration.events_callback_url`（指向 CC 事件入口，如 `http://127.0.0.1:8080/api/v1/integration/switch/events`）；事件落库后 POST 到该 URL。CC↔Switch **无鉴权**，依赖私有网络。完整流程见[对接说明](../docs/open-switch对接说明.md)与 [runbook](../docs/switch-standalone-runbook.md)。

**部署约束：单实例。** 通话 FSM、媒体房间、SIP registrar 与事件投递均在进程内存；多进程共享同一数据库会分裂状态，不支持双活。滚动升级请先排空通话或接受重启挂断。

视频录像需要 FFmpeg。在 `recordings.video` 中设置 `ffmpeg_path`（或将 FFmpeg 加入 `PATH`），
`format` 可选 `webm`（VP8/Opus）或 `mp4`（H.264/AAC）。纯音频与 IVR 提示音使用 `recordings.audio.dir`。
每通视频呼叫保存一份同时含画面和声音的文件。
下载接口可通过 `format=webm|mp4` 临时转换为另一种格式，不会长期保存第二份成品。

## 测试

```bash
go test -race ./... -count=1 -p 1
go vet ./...
go build -o bin/open-switch ./cmd/open-switch
cd third_party/media-sdk
go test -race . ./jitter ./mixer ./ring -count=1
```

应用音频、机器人坐席和语音通知的边界与升级步骤见 [应用音频与业务边界](../docs/应用音频与业务边界.md)。

数据库验收须设置指向专用 PostgreSQL 的 `OPEN_VOIP_TEST_DSN`；未设置时数据库测试跳过，不能记为迁移验收通过。历史迁移源码和原校验和保持不变：仅对核查过的损坏基线执行定点修复，并单独持久化 `execution_patch` / `execution_checksum`。已执行版本不重写，新录音字段和缺失运行字段通过增量迁移加入。

媒体组件锁定在 `go.mod` 和本地受控源码，详见 [SDK 修改清单](third_party/media-sdk/OPEN_VOIP_PATCHES.md)。本次验证原生库为 libsoxr 0.1.3、SpanDSP 0.0.6（Ubuntu 包）；安装版本用 `pkg-config --modversion soxr spandsp` 保存到构建记录。media-sdk 保留 Apache-2.0 许可证；libsoxr 与 SpanDSP 的 LGPL 等许可证及第三方文件声明应随发布包保留，分别核对原生包 `copyright` 文件。通话语义、监控及发布门禁见 [媒体契约](../docs/audio-quality.md)、[验收清单](../docs/audio-quality-qa.md) 和 [质量报告](../docs/audio-quality-report.md)。
