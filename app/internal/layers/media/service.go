// Package media 实现 L2 媒体层：SFU、录制、SIP 等，仅通过 MediaPort 对外暴露能力。
package media

import (
	"context"
	"time"

	"open-voip/internal/errs"
	"open-voip/internal/ports"
	"open-voip/internal/ports/dto"
)

// Service 是 MediaPort 的默认实现入口，Phase 0 为占位。
type Service struct{}

// NewService 创建媒体层服务。
func NewService() *Service {
	return &Service{}
}

var _ ports.MediaPort = (*Service)(nil)

func (s *Service) CreateRoom(ctx context.Context, callID string, opts dto.RoomOptions) error {
	return errs.ErrNotImplemented
}

func (s *Service) CloseRoom(ctx context.Context, callID string) error {
	return errs.ErrNotImplemented
}

func (s *Service) JoinWebRTC(ctx context.Context, callID, legID string, role dto.LegRole) (dto.LocalOffer, error) {
	return dto.LocalOffer{}, errs.ErrNotImplemented
}

func (s *Service) AcceptAnswer(ctx context.Context, callID, legID string, answerSDP string) error {
	return errs.ErrNotImplemented
}

func (s *Service) TrickleICE(ctx context.Context, callID, legID string, cand dto.ICECandidateInit) error {
	return errs.ErrNotImplemented
}

func (s *Service) IssueTURNCredentials(ctx context.Context, subject string, ttl time.Duration) (dto.TURNConfig, error) {
	return dto.TURNConfig{}, errs.ErrNotImplemented
}

func (s *Service) SetTrackMuted(ctx context.Context, callID, legID string, audio, video bool) error {
	return errs.ErrNotImplemented
}

func (s *Service) SetHold(ctx context.Context, callID, legID string, on bool) error {
	return errs.ErrNotImplemented
}

func (s *Service) RequestRenegotiation(ctx context.Context, callID, legID string, addVideo bool) error {
	return errs.ErrNotImplemented
}

func (s *Service) InjectAudio(ctx context.Context, callID, botLegID string, source dto.AudioSource) error {
	return errs.ErrNotImplemented
}

func (s *Service) SubscribeDTMF(ctx context.Context, callID, legID string, handler ports.DTMFHandler) error {
	return errs.ErrNotImplemented
}

func (s *Service) StartRecording(ctx context.Context, callID string, policy dto.RecordingPolicy) (string, error) {
	return "", errs.ErrNotImplemented
}

func (s *Service) StopRecording(ctx context.Context, recordingID string) error {
	return errs.ErrNotImplemented
}

func (s *Service) OriginateSIP(ctx context.Context, callID, legID, dial, trunkID string) error {
	return errs.ErrNotImplemented
}

func (s *Service) BridgeLegs(ctx context.Context, callID, legA, legB string) error {
	return errs.ErrNotImplemented
}
