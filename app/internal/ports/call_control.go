// Package ports 定义层间 Port 接口与事件类型，是全项目跨层协作的唯一契约入口。
package ports

import (
	"context"

	"open-voip/internal/ports/dto"
)

// CallControlPort 由 L3 实现，供 L4 Guest/外呼 与 app/http 调用；L4 不得持有 MediaPort。
type CallControlPort interface {
	// StartInbound 创建呼入通话并进入 IVR 或排队流程。
	// 调用方：L4、app。前置：队列存在且访客 token 有效（若适用）。
	StartInbound(ctx context.Context, req dto.InboundRequest) (callID string, err error)
	// Answer 坐席接听指定通话。
	Answer(ctx context.Context, callID, agentID string) error
	// Decline 坐席拒接，通话回到排队并释放坐席。
	Decline(ctx context.Context, callID, agentID string) error
	// Hangup 结束通话并驱动 FSM 至 ended。
	Hangup(ctx context.Context, callID string, reason dto.HangupReason) error
	// Transfer 盲转或咨询转。
	Transfer(ctx context.Context, callID string, req dto.TransferRequest) error
	// CompleteTransfer 结束咨询转：原坐席离席，客户与目标坐席继续。
	CompleteTransfer(ctx context.Context, callID string) error
	// Outbound 坐席发起外呼或分机互拨。
	Outbound(ctx context.Context, req dto.OutboundRequest) (callID string, err error)
	// StartIVR 为已有 Call 绑定 IVR 运行时（快照 ID）。
	StartIVR(ctx context.Context, callID, snapshotID string) error
	// Hold 保持或恢复。
	Hold(ctx context.Context, callID string, on bool) error
	// RequestVideo 一方请求升视频。
	RequestVideo(ctx context.Context, callID, fromLegID string) error
	// RespondVideo 对端同意或拒绝升视频。
	RespondVideo(ctx context.Context, callID string, accept bool) error
	// DowngradeVideo 降级为仅语音。
	DowngradeVideo(ctx context.Context, callID string) error
	// ScreenShare 开始或停止屏幕共享信令。
	ScreenShare(ctx context.Context, callID, legID string, on bool) error
	// ConferenceInvite 邀请第三人加入同一 Room。
	ConferenceInvite(ctx context.Context, callID, targetAgentID string) error
	// SupervisorListen 班长监听，返回 supervisor 腿 ID。
	SupervisorListen(ctx context.Context, callID, supervisorAgentID string) (legID string, err error)
	// SendDTMF 向指定腿发送 DTMF。
	SendDTMF(ctx context.Context, callID, legID, digit string) error
	// ForceReleaseAgent 班长强制释放坐席（挂断或等闲）。
	ForceReleaseAgent(ctx context.Context, agentID, policy string) error
	// GetCall 读取通话视图（L4 Guest 入队后取 leg，不经过 SignalingPort）。
	GetCall(ctx context.Context, callID string) (CallView, error)
}
