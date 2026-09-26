// Package external supplies inert business ports for controller-driven switching.
package external

import (
	"context"
	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"time"
)

// Null keeps the switch's media and call state independent of a business Platform API.
// Queue and agent workflows are unavailable in external mode; their decisions belong
// to the controller, while terminal control is driven by explicit call/leg commands.
type Null struct{}

func (Null) RequestAgent(context.Context, dto.DispatchRequest) (dto.DispatchResult, error) {
	return dto.DispatchResult{}, errs.NotFound("external 模式不提供 ACD")
}
func (Null) ByID(context.Context, string) (ports.AgentInfo, error) {
	return ports.AgentInfo{}, errs.NotFound("external 模式没有坐席目录")
}
func (Null) ByExtension(context.Context, string) (ports.AgentInfo, error) {
	return ports.AgentInfo{}, errs.NotFound("external 模式没有坐席目录")
}
func (Null) SetState(context.Context, string, string, string, string) error { return nil }
func (Null) GetQueue(context.Context, string) (ports.QueueSnapshot, error) {
	return ports.QueueSnapshot{}, errs.NotFound("external 模式没有队列")
}
func (Null) GetLatestIVR(context.Context, string) (ports.IVRSnapshot, error) {
	return ports.IVRSnapshot{}, errs.NotFound("external 模式没有 IVR")
}
func (Null) GetBusinessHours(context.Context, string) (ports.BusinessHours, error) {
	return ports.BusinessHours{WeekdayHours: "always"}, nil
}
func (Null) ResolveDID(context.Context, string) (string, error) {
	return "", errs.NotFound("external 模式没有 DID 路由")
}
func (Null) Now(context.Context) time.Time { return time.Now().UTC() }
func (Null) ForQueue(context.Context, string) (dto.RecordingPolicy, error) {
	return dto.RecordingPolicy{Mode: "off"}, nil
}
func (Null) Upsert(context.Context, ports.CDRWriteRequest) error { return nil }
func (Null) Save(context.Context, ports.RecordingMeta) error     { return nil }
