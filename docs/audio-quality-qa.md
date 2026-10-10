# 媒体质量验收与发布门禁

完整构建、测试和运行环境为 Linux/WSL，必须启用 CGO，并安装 libsoxr、SpanDSP 和 pkg-config。构建说明见 [Switch README](../open-switch/README.md)。测试数据库使用专用库，通过 `OPEN_VOIP_TEST_DSN` 注入，不将真实连接串写入报告。

## 自动回归

```bash
cd open-switch
go test -race ./... -count=1 -p 1
go test ./internal/layers/media -run 'TestPlayout|TestRTP|TestSIPWebRTC|TestBlockedLogging|TestConversation|TestNative' -v -count=1
cd third_party/media-sdk
go test -race . ./jitter ./mixer ./ring -count=1
```

两服务均须执行真实 PostgreSQL 迁移和投影测试，设置数据库时连接失败必须失败，不能跳过。Call 运行 `go test -race ./... -count=1 -p 1`。前端运行 `npm test`、`npm run lint`、`npm run build`。

验收包括 10/20/30/40 ms 分包、首包乱序、序号/时间戳回绕、真静音、重复包、SSRC/序号重启；0/20/40/80 ms 扰动、1%/3% 随机丢包和连续丢 3 包；恢复测试须断言变化后的真实输入重新到达，不能把 PLC 延续旧声音算作恢复。

`TestSIPWebRTCUnifiedBidirectionalRecording` 使用实际 Pion ICE/DTLS/SRTP 和 SIP UDP 接口，断言双向不同编码、排除自身声音、每个目标单一 RTP 连续输出、完整主录及等长分轨。它不包含物理话机、浏览器麦克风或运营商线路。

慢盘、磁盘满、队列溢出、停录超时、阻塞目标发送、应用不消费和阻塞日志均须独立注入，验证实时媒体继续推进。重采样与单次连续参考转换比较，100 Hz～3 kHz 通带幅度误差须 ≤1 dB。应用 generation 清理不能泄漏已取消声音。

## 容量及长稳

在专用 4 vCPU/8 GB Linux 基准机和实际部署环境分别执行，100 路双向、主录与两腿分轨全开，持续 60 分钟。每轮运行前保存代码版本、依赖版本、CPU 型号、内存限制、磁盘、网络扰动、起止时间及原始日志。

```bash
cd open-switch
go test -c -o /tmp/media-quality.test ./internal/layers/media
OPEN_VOIP_MEDIA_LOAD_DURATION=60m GOMAXPROCS=4 \
  taskset -c 0-3 /tmp/media-quality.test \
  -test.run '^TestMediaCapacity$' -test.v -test.timeout 75m
```

此测试使用真实 mixer 时钟、双编码、UDP 输出和 300 个 WAV，输入从收包后的媒体接口注入。4 核亲和性不等于独立 4 vCPU/8 GB 实例；WSL 组件结果不能代替部署容量。若在受限进程内运行，CPU/内存限制必须在启动前设置。输入发生器补齐合并 tick 并记录自身迟到；超过受控抖动条件的运行必须报告环境失格，不可记为门禁通过。

门禁：CPU P95 ≤70%；每个房间 20 ms 调度迟到 P99 ≤5 ms；无录音失败、发送溢出、持续媒体欠载或稳态无丢包 PLC。延迟 P95 ≤100 ms、P99 ≤140 ms，分别报告 Switch 新增处理、浏览器及线路延迟。短测和首音延迟只能作为初步证据，不能代替全程分布。

2 小时测试需输入时钟 ±100 ppm、缓冲有界、延迟无持续增长。`TestPlayoutTwoHourClockDriftSimulation` 推进 2 小时采样位置验证控制器，只是加速模拟；另须完成真实 2 小时运行。达不到 100 路时容量门禁失败，生产准入按实测安全容量下调，不降低音质要求。

```bash
OPEN_VOIP_MEDIA_LONG_DURATION=2h GOMAXPROCS=4 \
  /tmp/media-quality.test -test.run '^TestMediaWallClockDrift$' \
  -test.v -test.timeout 135m
```

真实时钟测试包含两路相反的 ±100 ppm 输入、UDP 输出、完整主录和分轨，停止后逐个核对 WAV 头的格式和共同样本长度。短时运行只验证测试工具，不视为 2 小时门禁通过。

`TestMediaContinuousLatencyWithJitter` 对连续输出窗口与已知伪随机输入相关匹配，分别测量 0/20/40 ms 扰动下的收包后媒体接口至 UDP 交付延迟。它补充首音指标；单通道测试不能替代部署环境中 100 路全程延迟采样。

## 真实端点及盲听

SIP↔SIP、SIP↔网页坐席、AI/应用 PCM 均覆盖双讲、保持恢复、转接、IVR 提示音切换、取消/尾帧及分轨对齐。记录 SDP、实际 PT/SSRC/时钟、服务统计、浏览器 WebRTC stats 和同一通话主录/分轨。

固定耳机、嘴距和语音样本，记录麦克风 `track.getSettings()`；对 AGC、回声消除等设备处理分别做对照。盲听清晰度、断续、首尾音、双讲和提示音过渡。只有分轨证据说明某一路底噪时才评估降噪，避免用全混音处理掩盖时序或编码缺陷。

## 灰度

全部门禁通过后按独立实例/呼入路由执行 5%→25%→100%，每档至少 30 分钟并覆盖核心场景。当前运行架构为单实例内存状态，同一数据库不能部署双活；灰度实例需要独立状态域和明确路由隔离。既有通话不迁移媒体实例。异常时停止新呼入并排空或回滚到上一发布包，保存失败样本和指标。
