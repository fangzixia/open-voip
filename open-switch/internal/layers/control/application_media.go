package control

import (
	"context"
	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

func (s *Service) OpenApplicationStream(ctx context.Context, callID, legID string, opts ports.MediaStreamOptions) (ports.ApplicationStream, error) {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	view, err := s.GetCall(ctx, callID)
	if err != nil {
		return nil, err
	}
	if view.State != "active" {
		return nil, errs.Conflict("仅已接通通话可接入音频", "")
	}
	leg, err := s.deps.Calls.GetLeg(ctx, callID, legID)
	if err != nil {
		return nil, err
	}
	if leg.Role != dto.LegRoleAgent && leg.Role != dto.LegRoleApplication {
		return nil, errs.InvalidRequest("PCM 会话需绑定 agent 或 application 腿")
	}
	m, ok := s.deps.Media.(ports.ApplicationMediaPort)
	if !ok {
		return nil, errs.NotImplemented("媒体流未配置")
	}
	return m.OpenApplicationStream(ctx, callID, legID, opts)
}

func (s *Service) GetLegPlayback(ctx context.Context, callID, legID, id string) (ports.PlaybackStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rt := s.calls[callID]; rt != nil {
		if p, ok := rt.playbacks[id]; ok && p.LegID == legID {
			return p, nil
		}
	}
	return ports.PlaybackStatus{}, errs.NotFound("播放任务不存在")
}

func (s *Service) playbackFinished(callID, legID, id, state string) {
	ctx, unlock := s.command(context.Background(), callID)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[callID]
	if rt == nil {
		s.mu.Unlock()
		return
	}
	p, ok := rt.playbacks[id]
	if !ok || p.State != "playing" {
		s.mu.Unlock()
		return
	}
	p.State = state
	rt.playbacks[id] = p
	s.mu.Unlock()
	_ = s.publishCall(ctx, callID, "leg.playback_"+state, "", map[string]any{"call_id": callID, "leg_id": legID, "playback_id": id, "completion_scope": "server_output"})
}
