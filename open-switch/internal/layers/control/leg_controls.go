package control

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/scope"
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
		if err := s.deps.Calls.DeleteLeg(ctx, callID, legID); err != nil {
			return err
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

// StartLegPlayback starts native playback to one existing leg.
func (s *Service) StartLegPlayback(ctx context.Context, callID, legID, assetID string) (string, error) {
	if assetID == "" {
		return "", errs.InvalidRequest("asset_id 必填")
	}
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	playbackID := uuid.New().String()
	if key := scope.Idempotency(ctx); key != "" {
		playbackID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(callID+"|"+legID+"|"+key)).String()
	}
	s.mu.Lock()
	rt := s.calls[callID]
	if rt != nil {
		if p, ok := rt.playbacks[playbackID]; ok {
			s.mu.Unlock()
			if p.AssetID != assetID {
				return "", errs.Conflict("幂等键已用于其他素材", "")
			}
			return playbackID, nil
		}
	}
	s.mu.Unlock()
	if err := s.guardExpectedVersion(ctx, callID); err != nil {
		return "", err
	}
	if rt != nil && len(rt.playbacks) >= 128 {
		return "", errs.Conflict("单通话播放任务超过上限", "")
	}
	if rt == nil || rt.rec.State != stateActive {
		return "", errs.Conflict("仅已接通通话可播放", "")
	}
	if _, err := s.deps.Calls.GetLeg(ctx, callID, legID); err != nil {
		return "", err
	}
	m, ok := s.deps.Media.(ports.ApplicationMediaPort)
	if !ok {
		return "", errs.NotImplemented("原生播放未配置")
	}
	err := m.StartPlayback(ctx, callID, legID, playbackID, assetID, func(state string) { s.playbackFinished(callID, legID, playbackID, state) })
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	if rt.playbacks == nil {
		rt.playbacks = map[string]ports.PlaybackStatus{}
	}
	rt.playbacks[playbackID] = ports.PlaybackStatus{ID: playbackID, LegID: legID, State: "playing", AssetID: assetID}
	s.mu.Unlock()
	if err := s.publishCall(ctx, callID, "leg.playback_started", "", map[string]any{"call_id": callID, "leg_id": legID, "playback_id": playbackID, "asset_id": assetID}); err != nil {
		_ = m.StopPlayback(ctx, callID, legID, playbackID)
		return "", err
	}
	return playbackID, nil
}

func (s *Service) StopLegPlayback(ctx context.Context, callID, legID, playbackID string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	if err := s.guardExpectedVersion(ctx, callID); err != nil {
		return err
	}
	if _, err := s.GetLegPlayback(ctx, callID, legID, playbackID); err != nil {
		return err
	}
	m, ok := s.deps.Media.(ports.ApplicationMediaPort)
	if !ok {
		return errs.NotImplemented("原生播放未配置")
	}
	return m.StopPlayback(ctx, callID, legID, playbackID)
}
