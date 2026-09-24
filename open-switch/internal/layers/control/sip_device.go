package control

import (
	"context"
	"github.com/google/uuid"
	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

type sipAnswerKey struct{}

// 由设备最终 SIP 响应（而非浏览器点击）完成 Offer 接听。
func (s *Service) ringDevice(ctx context.Context, callID, agentID string) error {
	info, err := s.deps.Agents.ByID(ctx, agentID)
	if err != nil {
		return err
	}
	if info.TerminalType != "sip" {
		return nil
	}
	if media, ok := s.deps.Media.(interface{ PrepareSIP(string) }); ok {
		media.PrepareSIP(callID)
	}
	if err := s.deps.Media.CreateRoom(ctx, callID, dto.RoomOptions{SessionType: dto.SessionTypeAudio}); err != nil {
		return err
	}
	legID := uuid.New().String()
	offerCtx, cancel := context.WithTimeout(context.Background(), offerTimeout)
	s.mu.Lock()
	rt := s.calls[callID]
	rt.sipOfferLeg = legID
	rt.sipOfferCancel = cancel
	s.mu.Unlock()
	go func() {
		defer cancel()
		err := s.deps.Media.OriginateSIP(offerCtx, callID, legID, info.SIPUsername, "@device")
		ctx, unlock := s.command(context.Background(), callID)
		defer unlock()
		s.mu.Lock()
		current := s.calls[callID]
		valid := current != nil && current.rec.State == stateRinging && current.offeredAgent == agentID && current.sipOfferLeg == legID
		s.mu.Unlock()
		if !valid {
			_ = s.deps.Media.LeaveRoom(ctx, callID, legID)
			return
		}
		if err == nil {
			err = s.Answer(context.WithValue(ctx, sipAnswerKey{}, true), callID, agentID)
		}
		if err != nil {
			_ = s.deps.Media.LeaveRoom(ctx, callID, legID)
			// 不可达设备标记为 busy，避免 ACD 反复振铃同一离线坐席。
			_ = s.setAgentState(ctx, callID, agentID, "", "busy", "sip_unavailable")
			s.mu.Lock()
			current.offeredAgent = ""
			current.sipOfferLeg = ""
			current.sipOfferCancel = nil
			s.mu.Unlock()
			if current.consultFrom != "" {
				from := current.consultFrom
				_ = s.deps.Media.SetHold(ctx, callID, customerLeg(current), false)
				s.removeLiveLeg(callID, legID)
				s.mu.Lock()
				current.activeAgent = from
				current.consultFrom = ""
				current.transferMode = ""
				current.held = false
				s.mu.Unlock()
				_ = s.transition(ctx, callID, stateActive)
				_ = s.publishCall(ctx, callID, "call.unhold", from, map[string]any{"call_id": callID, "reason": "consult_device_failed"})
			} else if current.rec.QueueID != nil {
				_ = s.transition(ctx, callID, stateQueued)
			} else {
				_ = s.Hangup(ctx, callID, dto.HangupReasonError)
			}
			_ = s.publishCall(ctx, callID, "call.device_failed", agentID, map[string]any{"call_id": callID})
		}
	}()
	return nil
}

// SIPSource 使用 SIP 栈已分配的 callID 发起已签入设备的呼叫。
func (s *Service) SIPSource(ctx context.Context, callID, extension, destination string) (string, string, error) {
	info, err := s.deps.Agents.ByExtension(ctx, extension)
	if err != nil {
		return "", "", err
	}
	if info.TerminalType != "sip" || info.SIPUsername != extension {
		return "", "", errDeviceBinding()
	}
	id, err := s.doOutboundWithID(ctx, dto.OutboundRequest{AgentID: info.AgentID, Destination: destination}, callID)
	if err != nil {
		return "", "", err
	}
	view, err := s.GetCall(ctx, id)
	if err != nil {
		return "", "", err
	}
	for _, leg := range view.Legs {
		if leg.AgentID == info.AgentID {
			return id, leg.ID, nil
		}
	}
	return "", "", errDeviceBinding()
}

var _ ports.CallControlPort = (*Service)(nil)

func errDeviceBinding() error { return errs.Forbidden("SIP 设备未绑定坐席分机") }
