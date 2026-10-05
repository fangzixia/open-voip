package media

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"open-switch/internal/observability"
)

func (s *Service) readPromptPCM(path string) ([]int16, int, []int16, int) {
	pcm, rate, err := readPCMWav(path)
	if err != nil {
		return nil, 0, nil, 0
	}
	hdPath := promptHDPath(path)
	if hdPath != path {
		if hd, hdRate, err := readPCMWav(hdPath); err == nil && len(hd) > 0 {
			return pcm, rate, hd, hdRate
		}
	}
	return pcm, rate, nil, 0
}

func promptHDPath(path string) string {
	if !strings.HasSuffix(strings.ToLower(path), ".wav") {
		return path
	}
	hd := strings.TrimSuffix(path, ".wav") + "_48k.wav"
	if _, err := os.Stat(hd); err == nil {
		return hd
	}
	return path
}

func (s *Service) roomNeedsG722(callID string) bool {
	r := s.getRoom(callID)
	if r == nil {
		return false
	}
	for _, rt := range r.snapshotSIPRTP() {
		if rt != nil && rt.currentCodec() == sipCodecG722 {
			return true
		}
	}
	return false
}

func (s *Service) emitPromptPlaySummary(ctx context.Context, callID string, frames int, lateMS int, codec string) {
	observability.Event(observability.WithFields(ctx, observability.Fields{CallID: callID}),
		"media", "prompt.play", "complete", "ok", "", time.Now(),
		"frames_sent", frames, "prompt_late_ms", lateMS, "codec_negotiated", codec)
	slog.Info("IVR 播放完成", "call_id", callID, "codec_negotiated", codec,
		"frames_sent", frames, "prompt_late_ms", lateMS)
}
