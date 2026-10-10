package media

import (
	"context"
	"testing"

	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

func TestClosedRoomCannotAcquireNewMediaResources(t *testing.T) {
	m := newScheduledRoomMixer()
	defer m.stop()
	r := &room{closed: true, mixAudio: true, mixer: m}
	s := &Service{rooms: map[string]*room{"closing": r}, audioRecDir: t.TempDir()}
	ctx := context.Background()
	if _, err := s.OpenApplicationStream(ctx, "closing", "agent", ports.MediaStreamOptions{Direction: "duplex", Input: ports.PCMFormat{SampleRate: 8000}, Output: ports.PCMFormat{SampleRate: 8000}}); err == nil {
		t.Fatal("application session attached after close")
	}
	if _, err := s.StartRecording(ctx, "closing", dto.RecordingPolicy{Mode: "audio"}); err == nil {
		t.Fatal("recording attached after close")
	}
	if _, err := s.JoinWebRTC(ctx, "closing", "agent", dto.LegRoleAgent); err == nil {
		t.Fatal("WebRTC endpoint attached after close")
	}
	if err := s.InjectAudio(ctx, "closing", "customer", dto.AudioSource{Loop: true}); err == nil {
		t.Fatal("prompt clock resumed after close")
	}
	s.playWaitingTone("closing", "customer", true, r.promptSeq.Load())
	if r.prompt != nil || len(s.recByID) != 0 || len(r.streams) != 0 || len(r.peers) != 0 {
		t.Fatal("late resource work retained a closed room")
	}
}
