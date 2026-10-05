package ports

import (
	"context"
	"time"

	"open-switch/internal/ports/dto"
)

// DTMFHandler 收到 DTMF 时的回调，由 L3 IVR 运行时注册。
type DTMFHandler func(ctx context.Context, digit dto.DTMFDigit)

// MediaPort 由 L2 实现，仅 L3 可调用；封装 SFU、录制、SIP 等媒体能力。
type MediaPort interface {
	// CreateRoom 创建与 call_id 同名的媒体 Room。
	CreateRoom(ctx context.Context, callID string, opts dto.RoomOptions) error
	// CloseRoom 释放 Room 与相关 PeerConnection。
	CloseRoom(ctx context.Context, callID string) error
	// JoinWebRTC 为指定 leg 创建 WebRTC 端并返回 Offer。
	JoinWebRTC(ctx context.Context, callID, legID string, role dto.LegRole) (dto.LocalOffer, error)
	// AcceptAnswer 设置远端 Answer SDP。
	AcceptAnswer(ctx context.Context, callID, legID string, answerSDP string) error
	// TrickleICE 追加 ICE candidate。
	TrickleICE(ctx context.Context, callID, legID string, cand dto.ICECandidateInit) error
	// IssueTURNCredentials 签发短期 TURN 凭证；未启用 TURN 时可返回零值。
	IssueTURNCredentials(ctx context.Context, subject string, ttl time.Duration) (dto.TURNConfig, error)
	// SetTrackMuted 静音/取消静音指定轨。
	SetTrackMuted(ctx context.Context, callID, legID string, audio, video bool) error
	// SetHold 保持或恢复，对端应听到 MOH 或静音策略。
	SetHold(ctx context.Context, callID, legID string, on bool) error
	// RequestRenegotiation 请求重协商（如升/降级视频）。
	RequestRenegotiation(ctx context.Context, callID, legID string, addVideo bool) error
	// InjectAudio 向 IVR bot leg 注入放音。
	InjectAudio(ctx context.Context, callID, botLegID string, source dto.AudioSource) error
	// StopInjectedAudio 停止当前队列或 IVR 放音。
	StopInjectedAudio(ctx context.Context, callID string) error
	// BeginQueueAnswerHandoff 排队接听：保留等待音 grace 后再淡出停止，并短暂屏蔽坐席→主叫媒体。
	BeginQueueAnswerHandoff(ctx context.Context, callID string, grace, fade time.Duration) error
	// SubscribeDTMF 订阅 leg 上的 DTMF 事件。
	SubscribeDTMF(ctx context.Context, callID, legID string, handler DTMFHandler) error
	// StartRecording 按策略开始录制，返回 recordingID。
	StartRecording(ctx context.Context, callID string, policy dto.RecordingPolicy) (recordingID string, err error)
	// SetRecordingMixInbound 控制 SIP 混音录音是否收录对端上行（IVR 阶段通常为 false）。
	SetRecordingMixInbound(ctx context.Context, callID string, enable bool) error
	// SetCallAudioProfile 设置通话音质档位（narrowband | wideband | hd_webrtc）。
	SetCallAudioProfile(ctx context.Context, callID, profile string) error
	// StopRecording 停止录制并落盘。
	StopRecording(ctx context.Context, recordingID string) error
	// OriginateSIP 发起 SIP leg（PSTN 可选模块）。
	OriginateSIP(ctx context.Context, callID, legID, dial, trunkID string) error
	// BridgeLegs 桥接两条 leg 的媒体。
	BridgeLegs(ctx context.Context, callID, legA, legB string) error
	// LeaveRoom 关闭指定腿的 PeerConnection，Room 可继续。
	LeaveRoom(ctx context.Context, callID, legID string) error
	// SendDTMF 向对端发送 RFC4733 DTMF。
	SendDTMF(ctx context.Context, callID, legID string, digit dto.DTMFDigit) error
	// RecordingInfo 读取录音元数据（停止后仍可查进程内缓存则可能为空）。
	RecordingInfo(ctx context.Context, recordingID string) (RecordingMeta, error)
	// PrepareSIP 在建房前声明该通话会有 SIP 腿加入，房间按 SIP 音频规则创建。
	PrepareSIP(callID string)
	// UnbridgeLegs 解除房间内的直连桥接，恢复为按房间转发。
	UnbridgeLegs(callID string)
	// DeferUntilAnswered 呼入 SIP 腿尚未发出 200 OK 时登记 fn 在应答后异步执行并返回 true；
	// 无需等待时返回 false 且不调用 fn。呼叫在应答前失败时登记的 fn 被丢弃。
	DeferUntilAnswered(callID string, fn func()) bool
	// PromptDuration 返回 IVR 素材（WAV）播放时长；素材无效时返回错误。
	PromptDuration(ctx context.Context, fileRef string) (time.Duration, error)
}
