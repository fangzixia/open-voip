package control

import (
	"context"

	"open-switch/internal/errs"
	"open-switch/internal/ports/dto"
)

// StartSurvey 坐席转满意度：释放坐席媒体，客户进入 post-call IVR。
func (s *Service) StartSurvey(ctx context.Context, callID, flowID string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	if err := s.guardExpectedVersion(ctx, callID); err != nil {
		return err
	}
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil {
		return errs.NotFound("通话不存在")
	}
	if rt.rec.State != stateActive && rt.rec.State != stateHeld {
		return errs.Conflict("当前状态无法转满意度", "")
	}
	if flowID == "" && rt.rec.QueueID != nil {
		q, err := s.deps.Config.GetQueue(ctx, *rt.rec.QueueID)
		if err == nil {
			flowID = q.PostCallIVRFlowID
		}
	}
	if flowID == "" {
		return errs.InvalidRequest("未配置满意度 IVR 流程")
	}
	agentID := rt.activeAgent
	if agentID != "" {
		_ = s.setAgentState(ctx, callID, agentID, "on_call", "acw", "survey")
	}
	_ = s.stopRecording(ctx, callID)
	_ = s.deps.Media.StopInjectedAudio(ctx, callID)
	s.mu.Lock()
	rt = s.calls[callID]
	if rt != nil {
		rt.activeAgent = ""
		rt.offeredAgent = ""
		keep := rt.legs[:0]
		for _, leg := range rt.legs {
			if leg.Role == dto.LegRoleCustomer || leg.Role == dto.LegRoleIVRBot {
				keep = append(keep, leg)
			}
		}
		rt.legs = keep
		rt.ivr = nil
	}
	s.mu.Unlock()
	if err := s.transition(ctx, callID, stateIVR); err != nil {
		return err
	}
	return s.bootIVR(ctx, callID, flowID)
}
