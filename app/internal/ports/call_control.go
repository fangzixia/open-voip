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
	// Hangup 结束通话并驱动 FSM 至 ended。
	Hangup(ctx context.Context, callID string, reason dto.HangupReason) error
	// Transfer 盲转或咨询转。
	Transfer(ctx context.Context, callID string, req dto.TransferRequest) error
	// Outbound 坐席发起外呼或分机互拨。
	Outbound(ctx context.Context, req dto.OutboundRequest) (callID string, err error)
	// StartIVR 为已有 Call 绑定 IVR 运行时（快照 ID）。
	StartIVR(ctx context.Context, callID, snapshotID string) error
}
