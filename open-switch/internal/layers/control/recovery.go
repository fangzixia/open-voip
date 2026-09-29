package control

import (
	"context"
	"log/slog"

	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"
)

// setAgentState 统一更新坐席状态并附带通话追踪信息。
func (s *Service) setAgentState(ctx context.Context, callID, agentID, from, to, reason string) error {
	return s.deps.Agents.SetCallState(ctx, callID, agentID, from, to, reason)
}

// 媒体会话无法跨进程存活。在接受新呼叫前先结束孤儿通话，
// 并可靠落库最终话单与坐席释放。
func (s *Service) Recover(ctx context.Context) error {
	source, ok := s.deps.Calls.(interface {
		Unfinished(context.Context) ([]ports.CallRecord, error)
	})
	if !ok {
		return nil
	}
	records, err := source.Unfinished(ctx)
	if err != nil {
		return err
	}
	for _, rec := range records {
		appCtx := scope.WithApplication(ctx, rec.ApplicationID)
		legs, err := s.deps.Calls.ListLegs(appCtx, rec.ID)
		if err != nil {
			return err
		}
		if rec.State == stateIVR && s.deps.IVRSessions != nil {
			sess, err := s.deps.IVRSessions.GetIVRSession(appCtx, rec.ID)
			if err == nil {
				if err := s.recoverIVRCall(appCtx, rec, legs, sess); err != nil {
					slog.Warn("IVR 恢复失败，将结束通话", "call_id", rec.ID, "error", err)
				} else {
					continue
				}
			}
		}
		if rec.State == stateQueued && rec.QueueID != nil {
			if err := s.recoverQueuedCall(appCtx, rec, legs); err != nil {
				slog.Warn("排队恢复失败，将结束通话", "call_id", rec.ID, "error", err)
			} else {
				continue
			}
		}
		if rec.State == stateRinging {
			if err := s.recoverRingingCall(appCtx, rec, legs); err != nil {
				slog.Warn("振铃恢复失败，将结束通话", "call_id", rec.ID, "error", err)
			} else {
				continue
			}
		}
		if rec.State == stateActive || rec.State == stateHeld || rec.State == stateTransferring {
			if err := s.recoverActiveCall(appCtx, rec, legs); err != nil {
				slog.Warn("已接通恢复失败，将结束通话", "call_id", rec.ID, "error", err)
			} else {
				continue
			}
		}
		rt := &runtimeCall{rec: rec, legs: legs, caller: rec.Caller, callee: rec.Callee, activeAgent: rec.AgentID, offeredAgent: rec.OfferedAgent, answeredAt: rec.AnsweredAt}
		s.mu.Lock()
		s.calls[rec.ID] = rt
		s.mu.Unlock()
		if err := s.Hangup(appCtx, rec.ID, dto.HangupReasonError); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Shutdown(ctx context.Context) error {
	calls, err := s.ListCalls(ctx)
	if err != nil {
		return err
	}
	for _, call := range calls {
		if err := s.Hangup(ctx, call.ID, dto.HangupReasonError); err != nil {
			return err
		}
	}
	return nil
}
