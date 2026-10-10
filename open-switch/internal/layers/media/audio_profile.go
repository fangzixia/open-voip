package media

import (
	"context"
	"log/slog"
	"time"

	"open-switch/internal/errs"
	"open-switch/internal/observability"
	"open-switch/internal/ports"
)

// SetCallAudioProfile 接受已验收的窄带通话策略。
func (s *Service) SetCallAudioProfile(ctx context.Context, callID, profile string) error {
	if profile != "" && profile != ports.AudioProfileNarrowband {
		return errs.InvalidRequest("当前媒体版本仅支持 narrowband")
	}
	_ = ctx
	s.mu.Lock()
	if s.callAudioProfile == nil {
		s.callAudioProfile = map[string]string{}
	}
	s.callAudioProfile[callID] = profile
	r := s.rooms[callID]
	s.mu.Unlock()
	applyProfileToRoom(r, profile)
	logAudioProfile(callID, profile)
	return nil
}

func logAudioProfile(callID, profile string) {
	if callID == "" {
		return
	}
	p := profile
	if p == "" {
		p = ports.AudioProfileNarrowband
	}
	ctx := observability.WithFields(context.Background(), observability.Fields{CallID: callID})
	observability.Event(ctx, "media", "audio.profile", "set", "ok", "", time.Now(),
		"audio_profile", p)
	slog.Info("通话音质档位已设置", "call_id", callID, "audio_profile", p)
}

func applyProfileToRoom(r *room, profile string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.audioProfile = ports.AudioProfileNarrowband
	r.mu.Unlock()
}

func (s *Service) roomAudioProfile(callID string) string {
	r := s.getRoom(callID)
	if r == nil {
		return ""
	}
	r.mu.RLock()
	p := r.audioProfile
	r.mu.RUnlock()
	return p
}
