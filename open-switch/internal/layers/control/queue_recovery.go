package control

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

func (s *Service) recoverQueuedCall(ctx context.Context, rec ports.CallRecord, legs []ports.CallLegRecord) error {
	if rec.QueueID == nil || *rec.QueueID == "" {
		return errs.InvalidRequest("排队通话缺少 queue_id")
	}
	q, err := s.deps.Config.GetQueue(ctx, *rec.QueueID)
	if err != nil {
		return err
	}
	rt := &runtimeCall{
		rec: rec, legs: legs, caller: rec.Caller, callee: rec.Callee,
		activeAgent: rec.AgentID, offeredAgent: rec.OfferedAgent, answeredAt: rec.AnsweredAt,
		queueName: q.Name, maxWait: time.Duration(q.MaxWaitSec) * time.Second, waitPrompt: q.WaitPrompt,
		queuedAt: rec.UpdatedAt,
	}
	if s.deps.Routing != nil {
		if rs, err := s.deps.Routing.Get(ctx, rec.ID); err == nil && rs.EnqueuedAt != nil {
			rt.queuedAt = *rs.EnqueuedAt
		}
	}
	s.mu.Lock()
	s.calls[rec.ID] = rt
	s.mu.Unlock()

	opts := dto.RoomOptions{SessionType: rec.SessionType, EnableVideo: rec.SessionType != dto.SessionTypeAudio}
	if err := s.deps.Media.CreateRoom(ctx, rec.ID, opts); err != nil {
		return err
	}
	promptFile := rt.waitPrompt
	if !strings.HasSuffix(strings.ToLower(promptFile), ".wav") {
		promptFile = ""
	}
	_ = s.deps.Media.InjectAudio(ctx, rec.ID, "", dto.AudioSource{FilePath: promptFile, Loop: true})
	s.publishPosition(ctx, rec.ID)
	if err := s.tryDispatch(ctx, rec.ID); err != nil {
		return err
	}
	slog.Info("排队会话已恢复", "call_id", rec.ID, "queue_id", *rec.QueueID)
	return nil
}

func (s *Service) recoverRingingCall(ctx context.Context, rec ports.CallRecord, legs []ports.CallLegRecord) error {
	agentID := rec.OfferedAgent
	if agentID == "" {
		agentID = rec.AgentID
	}
	if agentID == "" {
		return errs.InvalidRequest("振铃通话缺少坐席")
	}
	rt := &runtimeCall{
		rec: rec, legs: legs, caller: rec.Caller, callee: rec.Callee,
		activeAgent: rec.AgentID, offeredAgent: agentID, answeredAt: rec.AnsweredAt,
		offerAt: time.Now().UTC(),
	}
	if rec.QueueID != nil {
		if q, err := s.deps.Config.GetQueue(ctx, *rec.QueueID); err == nil {
			rt.queueName = q.Name
			rt.maxWait = time.Duration(q.MaxWaitSec) * time.Second
			rt.waitPrompt = q.WaitPrompt
		}
	}
	s.mu.Lock()
	s.calls[rec.ID] = rt
	s.mu.Unlock()

	opts := dto.RoomOptions{SessionType: rec.SessionType, EnableVideo: rec.SessionType != dto.SessionTypeAudio}
	if err := s.deps.Media.CreateRoom(ctx, rec.ID, opts); err != nil {
		return err
	}
	if err := s.ringDevice(ctx, rec.ID, agentID); err != nil {
		_ = s.setAgentState(ctx, rec.ID, agentID, "ringing", "idle", "recovery_ring_failed")
		s.emitAcdOfferFailed(ctx, rec.ID, agentID, rec.QueueID, "recovery_ring_failed")
		if err := s.transition(ctx, rec.ID, stateQueued); err != nil {
			return err
		}
		return s.recoverQueuedCall(ctx, rec, legs)
	}
	s.emitCall(ctx, rec.ID, "call.ringing", agentID, map[string]any{
		"call_id": rec.ID, "agent_id": agentID, "recovered": true,
	})
	slog.Info("振铃会话已恢复", "call_id", rec.ID, "agent_id", agentID)
	return nil
}

func (s *Service) emitAcdOfferFailed(ctx context.Context, callID, agentID string, queueID *string, reason string) {
	payload := map[string]any{"call_id": callID, "agent_id": agentID, "reason": reason}
	if queueID != nil {
		payload["queue_id"] = *queueID
	}
	s.emitCall(ctx, callID, "acd.offer_failed", agentID, payload)
}
