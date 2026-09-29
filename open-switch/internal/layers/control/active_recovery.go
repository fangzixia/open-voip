package control

import (
	"context"
	"log/slog"

	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

// recoverActiveCall 在 Switch 重启后保留已接通/保持/转接中的通话元数据，并提示客户端重建媒体。
func (s *Service) recoverActiveCall(ctx context.Context, rec ports.CallRecord, legs []ports.CallLegRecord) error {
	rt := &runtimeCall{
		rec: rec, legs: legs, caller: rec.Caller, callee: rec.Callee,
		activeAgent: rec.AgentID, offeredAgent: rec.OfferedAgent, answeredAt: rec.AnsweredAt,
		held: rec.State == stateHeld,
	}
	s.mu.Lock()
	s.calls[rec.ID] = rt
	s.mu.Unlock()

	opts := dto.RoomOptions{SessionType: rec.SessionType, EnableVideo: rec.SessionType != dto.SessionTypeAudio}
	if err := s.deps.Media.CreateRoom(ctx, rec.ID, opts); err != nil {
		return err
	}
	if rt.held {
		if cust := customerLeg(rt); cust != "" {
			if err := s.deps.Media.SetHold(ctx, rec.ID, cust, true); err != nil {
				slog.Warn("保持态恢复失败", "call_id", rec.ID, "error", err)
			}
		}
	}
	if rec.AgentID != "" {
		_ = s.setAgentState(ctx, rec.ID, rec.AgentID, "offline", "on_call", "recovery_active")
	}
	return s.publishCall(ctx, rec.ID, "call.media_reconnect_required", rec.AgentID, map[string]any{
		"call_id": rec.ID, "state": rec.State, "reason": "switch_restart",
	})
}
