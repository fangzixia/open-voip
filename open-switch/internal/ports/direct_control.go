package ports

import (
	"context"
	"open-switch/internal/ports/dto"
)

// DirectControlPort is the controller-driven call/leg command surface.
type DirectControlPort interface {
	CreateDirect(context.Context, dto.DirectCallRequest) (CallView, error)
	AddDirectLeg(context.Context, string, dto.DirectLegRequest) (CallView, error)
	DialDirectSIP(context.Context, string, dto.DirectSIPRequest) (CallView, error)
	BridgeDirect(context.Context, string, string, string) error
	LeaveDirectLeg(context.Context, string, string) error
	StartDirectRecording(context.Context, string, dto.RecordingPolicy) (RecordingMeta, error)
	StopDirectRecording(context.Context, string) (RecordingMeta, error)
}
