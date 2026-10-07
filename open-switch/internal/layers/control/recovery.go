package control

import (
	"context"
	"log/slog"
	"time"

	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

// setAgentState 统一更新坐席状态并附带通话追踪信息。
func (s *Service) setAgentState(ctx context.Context, callID, agentID, from, to, reason string) error {
	return s.deps.Agents.SetCallState(ctx, callID, agentID, from, to, reason)
}

// Recover 结束所有未终态通话。媒体无法跨进程恢复，不做 IVR/排队/振铃复活。
func (s *Service) Recover(ctx context.Context) error {
	records, err := s.deps.Calls.Unfinished(ctx)
	if err != nil {
		return err
	}
	for _, rec := range records {
		if err := s.failCall(ctx, rec.ID, dto.HangupReasonError, codeSwitchRecover, "交换服务恢复，通话已结束"); err != nil {
			slog.Warn("结束孤儿通话", "call_id", rec.ID, "err", err)
		}
	}
	return nil
}

// hangupPersistOnly 在无内存运行时时，仅通过持久化层结束通话（进程重启恢复路径）。
func (s *Service) hangupPersistOnly(ctx context.Context, callID string, rec ports.CallRecord, reason dto.HangupReason) error {
	if rec.State == stateEnded {
		return nil
	}
	now := time.Now().UTC()
	rec.State = stateEnded
	rec.EndedAt = &now
	rec.UpdatedAt = now
	result := "abandoned"
	if rec.AnsweredAt != nil {
		result = "answered"
	}
	if rec.AnsweredAt == nil && reason == dto.HangupReasonError {
		result = "failed"
	}
	if s.deps.Agents != nil {
		if rec.AgentID != "" {
			from := "on_call"
			if rec.AnsweredAt == nil {
				from = "ringing"
			}
			_ = s.setAgentState(ctx, callID, rec.AgentID, from, "idle", "hangup")
		}
		if rec.OfferedAgent != "" && rec.OfferedAgent != rec.AgentID {
			_ = s.setAgentState(ctx, callID, rec.OfferedAgent, "ringing", "idle", "hangup")
		}
	}
	if err := s.deps.Calls.UpdateCall(ctx, rec); err != nil {
		return err
	}
	if s.deps.CDR != nil {
		queueID := ""
		if rec.QueueID != nil {
			queueID = *rec.QueueID
		}
		_ = s.deps.CDR.Upsert(ctx, ports.CDRWriteRequest{
			CallID:      callID,
			Direction:   rec.Direction,
			QueueID:     queueID,
			AgentID:     rec.AgentID,
			Caller:      rec.Caller,
			Callee:      rec.Callee,
			SessionType: rec.SessionType,
			Result:      result,
			StartedAt:   rec.CreatedAt,
			AnsweredAt:  rec.AnsweredAt,
			EndedAt:     rec.EndedAt,
		})
	}
	if s.deps.Media != nil {
		_ = s.deps.Media.CloseRoom(ctx, callID)
	}
	endedPayload := map[string]any{
		"call_id": callID,
		"reason":  string(reason),
		"result":  result,
	}
	if result == "failed" {
		msg := defaultEndMessage(reason)
		if reason == dto.HangupReasonError {
			msg = "通话异常结束"
		}
		if msg != "" {
			endedPayload["message"] = msg
		}
		if reason == dto.HangupReasonError {
			endedPayload["error_code"] = codeCallState
		}
	}
	_ = s.publishCall(ctx, callID, "call.ended", rec.AgentID, endedPayload)
	return nil
}

func (s *Service) Shutdown(ctx context.Context) error {
	calls, err := s.ListCalls(ctx)
	if err != nil {
		return err
	}
	for _, call := range calls {
		if err := s.failCall(ctx, call.ID, dto.HangupReasonError, codeSwitchRecover, "服务关闭，通话已结束"); err != nil {
			return err
		}
	}
	return nil
}
