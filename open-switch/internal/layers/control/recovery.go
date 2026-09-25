package control

import (
	"context"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

// setAgentState 统一更新坐席状态并附带通话追踪信息。
func (s *Service) setAgentState(ctx context.Context, callID, agentID, from, to, reason string) error {
	if p, ok := s.deps.Agents.(interface {
		SetCallState(context.Context, string, string, string, string, string) error
	}); ok {
		return p.SetCallState(ctx, callID, agentID, from, to, reason)
	}
	return s.deps.Agents.SetState(ctx, agentID, from, to, reason)
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
		legs, err := s.deps.Calls.ListLegs(ctx, rec.ID)
		if err != nil {
			return err
		}
		rt := &runtimeCall{rec: rec, legs: legs, caller: rec.Caller, callee: rec.Callee, activeAgent: rec.AgentID, offeredAgent: rec.OfferedAgent, answeredAt: rec.AnsweredAt}
		s.mu.Lock()
		s.calls[rec.ID] = rt
		s.mu.Unlock()
		if err := s.Hangup(ctx, rec.ID, dto.HangupReasonError); err != nil {
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
