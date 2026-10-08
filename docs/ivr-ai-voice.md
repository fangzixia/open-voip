# 人工坐席、机器人坐席与语音通知

完整边界、接口和升级步骤见 [应用音频与业务边界](./应用音频与业务边界.md)。

## 运行方式

| 类型 | call 业务 | 音频连接 | 媒体执行 |
| --- | --- | --- | --- |
| 人工坐席 | 页面操作、权限、业务资料 | 浏览器 WebRTC，话机 SIP/RTP | 终端设备处理音频；Switch 控制通话及媒体 |
| 机器人坐席 | 模型、提示词、对话、打断、用量 | 双向 PCM WebSocket | Switch 重采样、缓存、编解码、20 ms 发送和混音 |
| 固定素材通知 | 持久化任务、内容、结果、取消和结束通话 | 通用 SIP 拨号及播放 API | Switch 读取 WAV、转换、实时播放及报告输出结束 |

通知不占发起人坐席容量，不需要签入，不要求发起人与机器人是同一账号，也不依赖 `aibot.enabled`。通知由 `internal/notification` 执行，机器人由 `internal/aibot` 执行，仍在一个 call 进程中运行。

## 机器人配置与 IVR

见 [配置示例](../open-call/deploy/config.example.yml)。`aibot.agent_id` 是参与 Switch ACD 的虚拟坐席，`queue_ids` 是签入队列。配置模型网关、密钥、模型、声音及提示词即可；已删除 Worker 登录账号、密码及 `wideband_webrtc`，无需为 Worker 配置 TURN/ICE。

`PUT /api/v1/aibot/queues/{queueId}/profile` 保存队列提示词，非空时覆盖 `aibot.system_prompt`。AI 用量仍使用 `ai.session.completed` webhook。

IVR 示例：欢迎语 `play` → `route_queue` 到机器人所在队列。Switch 负责排队、坐席预留和振铃，call Worker 接听，再建立 PCM 会话。真人与机器人可按产品配置参与队列，容量仍由 Switch 决定。

## 通知接口

`POST /api/v1/calls/voice-notifications` 提交 `destination`、`prompt_asset_id`、可选 `trunk_id`。HTTP 202 返回业务任务，`id` 同时关联 Switch call ID；返回值不再是占用人工坐席的 CallView。

`GET /api/v1/calls/voice-notifications/{taskId}` 查询本人任务，`DELETE` 请求取消。坐席工作台支持刷新最近任务及取消未结束任务。任务及幂等键保存在 call 数据库；关闭页面不会取消任务。

通知完成只表示素材已从服务器输出并经过短暂尾音余量，不代表客户听见或理解。未播放完时客户挂断、素材无效、拨号失败不能计为完成。

## 联调

1. 关闭 AI，使用有通知权限的 WebRTC 或 SIP 坐席提交通知，确认发起人状态不变。
2. 核对任务状态、尾音及 BYE；测试无人接听、拒接、播放中挂断、取消。
3. 启用机器人，测试 SIP/WebRTC 客户入队、应答及双向音频。
4. 测试打断、迟到模型音频、模型断线、转人工及挂断；转交后旧 Worker 不得结束后续通话。
5. 回归人工接听、保持、转接、录音和设备操作；真实媒体验收见 [音质检查](./audio-quality.md)。
