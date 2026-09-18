// Package control 实现 L3 信令与呼叫控制：Call FSM、IVR 运行时、媒体编排时机。
package control

import (
	"context"

	"open-voip/internal/errs"
	"open-voip/internal/ports"
	"open-voip/internal/ports/dto"
)

// Deps 为 L3 服务依赖的 Port，由 bootstrap 注入。
type Deps struct {
	// Media 媒体层 Port，仅 L3 可调用。
	Media ports.MediaPort
	// ACD 坐席分配 Port。
	ACD ports.ACDDispatchPort
	// Config 配置快照 Port。
	Config ports.ConfigSnapshotPort
	// Agents 坐席目录 Port。
	Agents ports.AgentDirectoryPort
	// RecordingPolicy 录音策略 Port。
	RecordingPolicy ports.RecordingPolicyPort
	// CDR 话单 Port。
	CDR ports.CDRRecorderPort
	// CallEvents 呼叫事件发布。
	CallEvents ports.CallEventPublisher
}

// Service 实现 CallControlPort。
type Service struct {
	deps Deps
}

// NewService 创建呼叫控制服务。
func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

var _ ports.CallControlPort = (*Service)(nil)

func (s *Service) StartInbound(ctx context.Context, req dto.InboundRequest) (string, error) {
	return "", errs.ErrNotImplemented
}

func (s *Service) Answer(ctx context.Context, callID, agentID string) error {
	return errs.ErrNotImplemented
}

func (s *Service) Hangup(ctx context.Context, callID string, reason dto.HangupReason) error {
	return errs.ErrNotImplemented
}

func (s *Service) Transfer(ctx context.Context, callID string, req dto.TransferRequest) error {
	return errs.ErrNotImplemented
}

func (s *Service) Outbound(ctx context.Context, req dto.OutboundRequest) (string, error) {
	return "", errs.ErrNotImplemented
}

func (s *Service) StartIVR(ctx context.Context, callID, snapshotID string) error {
	return errs.ErrNotImplemented
}

// ActiveCalls 返回当前活跃通话数，供 status API；Phase 0 恒为 0。
func (s *Service) ActiveCalls() int {
	return 0
}
