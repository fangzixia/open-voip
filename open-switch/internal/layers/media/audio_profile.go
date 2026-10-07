package media

import (
	"context"
	"log/slog"
	"time"

	"open-switch/internal/observability"
	"open-switch/internal/ports"
)

// SetCallAudioProfile 按队列 audio_profile 设置通话音质策略（宽带 SIP / WebRTC HD）。
func (s *Service) SetCallAudioProfile(ctx context.Context, callID, profile string) error {
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
	preferWB := p == ports.AudioProfileWideband || p == ports.AudioProfileHDWebRTC ||
		p == "wideband" || p == "hd_webrtc"
	preferOpus := p == ports.AudioProfileHDWebRTC || p == "hd_webrtc"
	ctx := observability.WithFields(context.Background(), observability.Fields{CallID: callID})
	observability.Event(ctx, "media", "audio.profile", "set", "ok", "", time.Now(),
		"audio_profile", p, "prefer_wideband", preferWB, "prefer_opus", preferOpus)
	slog.Info("通话音质档位已设置", "call_id", callID, "audio_profile", p,
		"prefer_wideband", preferWB, "prefer_opus", preferOpus)
}

func applyProfileToRoom(r *room, profile string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if profile == "" || profile == ports.AudioProfileNarrowband || profile == "narrowband" {
		r.audioProfile = ports.AudioProfileNarrowband
	} else {
		r.audioProfile = profile
	}
	r.preferWideband = false
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

func (s *Service) roomPrefersHD(callID string) bool {
	p := s.roomAudioProfile(callID)
	return p == ports.AudioProfileHDWebRTC || p == "hd_webrtc"
}

// sdpNegotiatePrefs 返回 SIP 应答是否优先宽带 / Opus（队列 audio_profile + 全局配置）。
func (s *Service) sdpNegotiatePrefs(callID string) (preferWideband, preferOpus bool) {
	r := s.getRoom(callID)
	if r == nil {
		return false, false
	}
	r.mu.RLock()
	p := r.audioProfile
	preferWideband = r.preferWideband
	preferOpus = p == ports.AudioProfileHDWebRTC || p == "hd_webrtc"
	r.mu.RUnlock()
	return preferWideband, preferOpus
}
