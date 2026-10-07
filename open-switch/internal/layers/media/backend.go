package media

import (
	"context"

	"open-switch/internal/ports/dto"
)

// MediaBackend 媒体会话抽象，便于后续接入 FreeSWITCH / rtpengine 等外部后端。
type MediaBackend interface {
	CreateSession(ctx context.Context, callID string, opts dto.RoomOptions) error
	CloseSession(ctx context.Context, callID string) error
	StartRecording(ctx context.Context, callID string, policy dto.RecordingPolicy) (string, error)
	StopRecording(ctx context.Context, recordingID string) error
	BridgeLegs(ctx context.Context, callID, legA, legB string) error
}

type inProcessBackend struct {
	svc *Service
}

func newInProcessBackend(s *Service) MediaBackend {
	if s == nil {
		return nil
	}
	return &inProcessBackend{svc: s}
}

func (b *inProcessBackend) CreateSession(ctx context.Context, callID string, opts dto.RoomOptions) error {
	return b.svc.CreateRoom(ctx, callID, opts)
}

func (b *inProcessBackend) CloseSession(ctx context.Context, callID string) error {
	return b.svc.CloseRoom(ctx, callID)
}

func (b *inProcessBackend) StartRecording(ctx context.Context, callID string, policy dto.RecordingPolicy) (string, error) {
	return b.svc.StartRecording(ctx, callID, policy)
}

func (b *inProcessBackend) StopRecording(ctx context.Context, recordingID string) error {
	return b.svc.StopRecording(ctx, recordingID)
}

func (b *inProcessBackend) BridgeLegs(ctx context.Context, callID, legA, legB string) error {
	return b.svc.BridgeLegs(ctx, callID, legA, legB)
}
