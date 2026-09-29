package ports

import (
	"context"
	"open-switch/internal/ports/dto"
)

// DirectSIPResult 异步 SIP 拨号命令的接受结果。
type DirectSIPResult struct {
	View      CallView `json:"call"`
	CommandID string   `json:"command_id"`
	LegID     string   `json:"leg_id"`
}

// DirectControlPort 由外部控制器驱动的通话/通话腿命令接口。
type DirectControlPort interface {
	CreateStubCall(context.Context, dto.StubCallRequest) (CallView, error)
	CreateDirect(context.Context, dto.DirectCallRequest) (CallView, error)
	AddDirectLeg(context.Context, string, dto.DirectLegRequest) (CallView, error)
	DialDirectSIP(context.Context, string, dto.DirectSIPRequest) (DirectSIPResult, error)
	BridgeDirect(context.Context, string, string, string) error
	ReplaceBridge(context.Context, string, string, string, string) error
	EndBridge(context.Context, string, string) error
	LeaveDirectLeg(context.Context, string, string) error
	HoldLeg(context.Context, string, string, bool) error
	RejectLeg(context.Context, string, string, string) error
	StartLegPlayback(context.Context, string, string, string) (string, error)
	StopLegPlayback(context.Context, string, string, string) error
	StartDirectRecording(context.Context, string, dto.RecordingPolicy) (RecordingMeta, error)
	StopDirectRecording(context.Context, string) (RecordingMeta, error)
}
