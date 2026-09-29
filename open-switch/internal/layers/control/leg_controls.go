package control

import (
	"context"
	"fmt"

	"uuid"

	"open-switch/internal/errs"
	"open-switch/internal/ports/dto"
)

// HoldLeg 对单条媒体腿保持/恢复（直控与编排场景共用）。
func (s *Service) HoldLeg(ctx context.Context, callID, legID string, on bool) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	if err := s.guardExpectedVersion(ctx, callID); err != nil {
		return err
	}
	hash := fmt.Sprintf("leg.hold|%s|%v", legID, on)
	return s.runIdempotentMutation(ctx, callID, "leg.hold", hash, func() error {
		if _, err := s.deps.Calls.GetLeg(ctx, callID, legID); err != nil {
			return err
		}
		if err := s.deps.Media.SetHold(ctx, callID, legID, on); err != nil {
			return err
		}
		ev := "leg.unhold"
		if on {
			ev = "leg.hold"
		}
		return s.publishCall(ctx, callID, ev, "", map[string]any{"call_id": callID, "leg_id": legID})
	})
}

// RejectLeg 拒绝仍等待业务动作的入呼腿（幂等移除腿）。
func (s *Service) RejectLeg(ctx context.Context, callID, legID, reason string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	if err := s.guardExpectedVersion(ctx, callID); err != nil {
		return err
	}
	hash := fmt.Sprintf("leg.reject|%s|%s", legID, reason)
	return s.runIdempotentMutation(ctx, callID, "leg.reject", hash, func() error {
		leg, err := s.deps.Calls.GetLeg(ctx, callID, legID)
		if err != nil {
			return err
		}
		if err := s.deps.Media.LeaveRoom(ctx, callID, legID); err != nil {
			if api := errs.AsAPIError(err); api == nil || api.HTTP != 404 {
				return err
			}
		}
		if remover, ok := s.deps.Calls.(interface {
			DeleteLeg(context.Context, string, string) error
		}); ok {
			if err := remover.DeleteLeg(ctx, callID, legID); err != nil {
				return err
			}
		}
		s.mu.Lock()
		if rt := s.calls[callID]; rt != nil {
			for i, l := range rt.legs {
				if l.ID == legID {
					rt.legs = append(rt.legs[:i], rt.legs[i+1:]...)
					break
				}
			}
		}
		s.mu.Unlock()
		if err := s.publishCall(ctx, callID, "leg.rejected", "", map[string]any{
			"call_id": callID, "leg_id": legID, "reason": reason, "role": leg.Role,
		}); err != nil {
			return err
		}
		return nil
	})
}

// StartLegPlayback 在指定腿上播放已上传 IVR 素材（asset_id 为素材文件名/ID）。
func (s *Service) StartLegPlayback(ctx context.Context, callID, legID, assetID string) (string, error) {
	if assetID == "" {
		return "", errs.InvalidRequest("asset_id 必填")
	}
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	if err := s.guardExpectedVersion(ctx, callID); err != nil {
		return "", err
	}
	playbackID := uuid.New().String()
	hash := fmt.Sprintf("leg.playback|%s|%s", legID, assetID)
	var runErr error
	err := s.runIdempotentMutation(ctx, callID, "leg.playback", hash, func() error {
		if _, err := s.deps.Calls.GetLeg(ctx, callID, legID); err != nil {
			runErr = err
			return err
		}
		if err := s.deps.Media.InjectAudio(ctx, callID, legID, dto.AudioSource{FilePath: assetID, Loop: false}); err != nil {
			runErr = err
			return err
		}
		runErr = nil
		return s.publishCall(ctx, callID, "leg.playback_started", "", map[string]any{
			"call_id": callID, "leg_id": legID, "playback_id": playbackID, "asset_id": assetID,
		})
	})
	if err != nil {
		return "", err
	}
	if runErr != nil {
		return "", runErr
	}
	s.mu.Lock()
	if rt := s.calls[callID]; rt != nil {
		if rt.playbacks == nil {
			rt.playbacks = map[string]string{}
		}
		rt.playbacks[playbackID] = legID
	}
	s.mu.Unlock()
	return playbackID, nil
}

// StopLegPlayback 停止腿上放音（当前实现为停止该通话房间内的注入音频）。
func (s *Service) StopLegPlayback(ctx context.Context, callID, legID, playbackID string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	hash := fmt.Sprintf("leg.playback.stop|%s|%s", legID, playbackID)
	return s.runIdempotentMutation(ctx, callID, "leg.playback.stop", hash, func() error {
		s.mu.Lock()
		rt := s.calls[callID]
		if rt != nil && rt.playbacks != nil {
			delete(rt.playbacks, playbackID)
		}
		s.mu.Unlock()
		if err := s.deps.Media.StopInjectedAudio(ctx, callID); err != nil {
			return err
		}
		return s.publishCall(ctx, callID, "leg.playback_stopped", "", map[string]any{
			"call_id": callID, "leg_id": legID, "playback_id": playbackID,
		})
	})
}
