package control

import (
	"context"
	"sort"
	"sync"
	"time"

	"open-switch/internal/observability"
	"open-switch/internal/ports/dto"
)

const tickParallelism = 8

// Run 后台派单与超时。
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

// tick 按优先级和入队时间先处理等待通话，再扫描其他状态的超时。
func (s *Service) tick(ctx context.Context) {
	type queuedItem struct {
		id     string
		prio   int
		queued time.Time
	}
	s.mu.Lock()
	queued := make([]queuedItem, 0)
	ids := make([]string, 0, len(s.calls))
	for id, rt := range s.calls {
		if rt.rec.State == stateQueued {
			queued = append(queued, queuedItem{id: id, prio: rt.rec.Priority, queued: rt.queuedAt})
			continue
		}
		ids = append(ids, id)
	}
	s.mu.Unlock()
	sort.Slice(queued, func(i, j int) bool {
		if queued[i].prio != queued[j].prio {
			return queued[i].prio > queued[j].prio
		}
		return queued[i].queued.Before(queued[j].queued)
	})
	now := time.Now().UTC()
	for _, q := range queued {
		s.tickCall(ctx, q.id, now)
	}
	sem := make(chan struct{}, tickParallelism)
	var wg sync.WaitGroup
	for _, id := range ids {
		sem <- struct{}{}
		wg.Add(1)
		go func(id string) {
			defer func() { <-sem; wg.Done() }()
			s.tickCall(ctx, id, now)
		}(id)
	}
	wg.Wait()
}

// tickCall 推进单通呼叫的排队、IVR 或振铃超时状态。
func (s *Service) tickCall(ctx context.Context, id string, now time.Time) {
	ctx, unlock := s.command(ctx, id)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[id]
	s.mu.Unlock()
	if rt == nil {
		return
	}
	switch rt.rec.State {
	case stateQueued:
		if now.Sub(rt.queuedAt) > rt.maxWait {
			if s.overflow(ctx, id) {
				return
			}
			_ = s.Hangup(ctx, id, dto.HangupReasonTimeout)
			return
		}
		s.publishPosition(ctx, id)
		_ = s.tryDispatch(ctx, id)
	case stateIVR:
		s.tickIVR(ctx, id)
	case stateRinging:
		if !rt.offerAt.IsZero() && now.Sub(rt.offerAt) > offerTimeout {
			if rt.sipOfferCancel != nil {
				rt.sipOfferCancel()
				continueSIP := rt.sipOfferLeg != ""
				if continueSIP {
					return
				}
			}
			agentID := rt.offeredAgent
			if agentID != "" {
				_ = s.setAgentState(ctx, id, agentID, "ringing", "idle", "offer_timeout")
				s.emitAcdOfferFailed(ctx, id, agentID, rt.rec.QueueID, "offer_timeout")
			}
			s.mu.Lock()
			consultFrom := ""
			if cur := s.calls[id]; cur != nil {
				consultFrom = cur.consultFrom
				cur.offeredAgent = ""
			}
			s.mu.Unlock()
			if consultFrom != "" {
				cust := customerLeg(rt)
				_ = s.deps.Media.SetHold(ctx, id, cust, false)
				s.mu.Lock()
				if cur := s.calls[id]; cur != nil {
					cur.consultFrom = ""
					cur.transferMode = ""
					cur.held = false
				}
				s.mu.Unlock()
				_ = s.transition(ctx, id, stateActive)
				_ = s.publishCall(ctx, id, "call.unhold", consultFrom, map[string]any{"call_id": id, "reason": "consult_timeout"})
				return
			}
			_ = s.transition(ctx, id, stateQueued)
			_ = s.tryDispatch(ctx, id)
		}
	}
}

// tryDispatch 请求本地 ACD 原子选人；旧派单结果会释放坐席，避免占用已变化的通话。
func (s *Service) tryDispatch(ctx context.Context, callID string) error {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.State != stateQueued || rt.rec.QueueID == nil {
		return nil
	}
	started := time.Now()
	res, err := s.deps.ACD.RequestAgent(ctx, dto.DispatchRequest{
		CallID:       callID,
		QueueID:      *rt.rec.QueueID,
		RequireVideo: rt.rec.SessionType == dto.SessionTypeVideo,
		SkillIDs:     nil,
	})
	if err != nil {
		observability.Event(ctx, "acd", "acd.dispatch", "response", "error", "dispatch_failed", started, "queue_id", *rt.rec.QueueID)
		return err
	}
	observability.Event(ctx, "acd", "acd.dispatch", "response", "ok", "", started, "queue_id", *rt.rec.QueueID, "agent_id", res.AgentID)
	if res.AgentID == "" {
		return nil
	}
	s.mu.Lock()
	rt = s.calls[callID]
	if rt == nil || rt.rec.State != stateQueued {
		s.mu.Unlock()
		_ = s.setAgentState(ctx, callID, res.AgentID, "ringing", "idle", "stale_offer")
		return nil
	}
	rt.offeredAgent = res.AgentID
	rt.offerAt = time.Now().UTC()
	s.mu.Unlock()
	if err := s.transition(ctx, callID, stateRinging); err != nil {
		return err
	}
	if err := s.ringDevice(ctx, callID, res.AgentID); err != nil {
		_ = s.setAgentState(ctx, callID, res.AgentID, "ringing", "idle", "ring_failed")
		s.emitAcdOfferFailed(ctx, callID, res.AgentID, rt.rec.QueueID, "ring_failed")
		_ = s.transition(ctx, callID, stateQueued)
		return s.tryDispatch(ctx, callID)
	}
	ringPayload := map[string]any{
		"call_id":      callID,
		"queue_id":     deref(rt.rec.QueueID),
		"queue_name":   rt.queueName,
		"caller":       rt.caller,
		"session_type": string(rt.rec.SessionType),
		"agent_id":     res.AgentID,
	}
	s.emitCall(ctx, callID, "call.ringing", res.AgentID, ringPayload)
	s.emitCall(ctx, callID, "leg.ringing", res.AgentID, ringPayload)
	return nil
}
