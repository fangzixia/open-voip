package control

import (
	"context"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"
)

// EnterIVR 释放服务方通话腿，保留客户媒体并执行指定流程；不解释流程的业务用途。
func (s *Service) EnterIVR(ctx context.Context, callID, flowID string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	return s.runIdempotentMutation(ctx, callID, "ivr.enter", "ivr.enter|"+flowID, func() error {
		return s.enterIVROnce(ctx, callID, flowID)
	})
}

func (s *Service) enterIVROnce(ctx context.Context, callID, flowID string) error {
	if err := s.guardExpectedVersion(ctx, callID); err != nil {
		return err
	}
	// 后续节点属于本命令内部动作，不重复使用入口的版本约束与幂等键。
	ctx = scope.WithoutMutation(ctx)
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil {
		return errs.NotFound("通话不存在")
	}
	if rt.rec.State != stateActive && rt.rec.State != stateHeld {
		return errs.Conflict("当前状态无法进入后续 IVR", "")
	}
	if flowID == "" {
		return errs.InvalidRequest("flow_id 必填")
	}
	snap, err := s.deps.Config.GetLatestIVR(ctx, flowID)
	if err != nil {
		return err
	}
	if _, err := parseIVRPayload(snap.PayloadJSON); err != nil {
		return err
	}
	remoteLeg := ""
	for _, leg := range rt.legs {
		if leg.Role == dto.LegRoleCustomer || leg.Role == dto.LegRolePSTN {
			remoteLeg = leg.ID
			break
		}
	}
	if remoteLeg == "" {
		return errs.InvalidRequest("通话缺少客户腿")
	}
	if rt.held {
		if err := s.deps.Media.SetHold(ctx, callID, remoteLeg, false); err != nil {
			return err
		}
		s.mu.Lock()
		rt.held = false
		s.mu.Unlock()
	}
	if err := s.stopRecording(ctx, callID); err != nil {
		return err
	}
	if err := s.deps.Media.StopInjectedAudio(ctx, callID); err != nil {
		return err
	}
	for _, leg := range append([]ports.CallLegRecord(nil), rt.legs...) {
		if leg.Role == dto.LegRoleCustomer || leg.Role == dto.LegRolePSTN || leg.Role == dto.LegRoleIVRBot {
			continue
		}
		if err := s.deps.Media.LeaveRoom(ctx, callID, leg.ID); err != nil {
			return err
		}
		if err := s.deps.Calls.DeleteLeg(ctx, callID, leg.ID); err != nil {
			return err
		}
		if leg.Role == dto.LegRoleAgent && leg.AgentID != nil && *leg.AgentID != "" {
			if err := s.setAgentState(ctx, callID, *leg.AgentID, "on_call", "acw", "ivr.enter"); err != nil {
				return err
			}
		}
		s.removeLiveLeg(callID, leg.ID)
	}
	s.mu.Lock()
	rt = s.calls[callID]
	if rt != nil {
		rt.activeAgent = ""
		rt.offeredAgent = ""
		rt.ivr = nil
	}
	s.mu.Unlock()
	return s.startIVRPayload(ctx, callID, snap)
}
