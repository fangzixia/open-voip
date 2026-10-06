package aibot

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"open-call/internal/aibot/realtime"
	"open-call/internal/config"
)

var (
	errNoAgentLeg = errors.New("缺少 agent 通话腿")
	errNoPSTNLeg  = errors.New("缺少 PSTN 通话腿")
)

// RunInboundAI AI 呼入全双工会话。
func RunInboundAI(ctx context.Context, sig *Signaling, cfg config.AibotConfig, callID, mediaAgentID string, usage *UsageRecorder, log *slog.Logger) error {
	if err := sig.api.Answer(ctx, callID, mediaAgentID); err != nil {
		return err
	}
	view, err := sig.api.GetCall(ctx, callID)
	if err != nil {
		return err
	}
	agentLeg, ok := findAgentLeg(view, mediaAgentID)
	if !ok {
		return errNoAgentLeg
	}
	peer, err := sig.Connect(ctx, callID, agentLeg, mediaAgentID)
	if err != nil {
		return err
	}
	defer peer.Close()

	rs, err := realtime.Connect(ctx, cfg.OpenAI, cfg.SystemPrompt)
	if err != nil {
		return err
	}
	defer func() { _ = rs.Close() }()

	var inputMs, outputMs atomic.Int64
	start := time.Now()

	rs.OnAudio(func(pcm []byte) {
		samples := realtime.PCM16BytesToSamples(pcm)
		samples8k := realtime.ResampleSimple(samples, realtime.RealtimeOutputRate, 8000)
		outputMs.Add(int64(len(samples8k)) * 1000 / 8000)
		_ = peer.WritePCMU8k(samples8k)
	})

	peer.SetRemotePCMHandler(func(pcm []int16, rate int) {
		pcm24 := realtime.ResampleSimple(pcm, rate, realtime.RealtimeInputRate)
		inputMs.Add(int64(len(pcm24)) * 1000 / int64(rate))
		raw := samplesToLE(pcm24)
		_ = rs.AppendInputPCM(ctx, raw)
	})

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if usage != nil {
				usage.RecordAISession(callID, mediaAgentID, view.QueueID, start, inputMs.Load(), outputMs.Load())
			}
			return ctx.Err()
		case <-ticker.C:
			v, err := sig.api.GetCall(ctx, callID)
			if err != nil {
				continue
			}
			if v.State == "ended" {
				if usage != nil {
					usage.RecordAISession(callID, mediaAgentID, v.QueueID, start, inputMs.Load(), outputMs.Load())
				}
				return nil
			}
		}
	}
}

func samplesToLE(samples []int16) []byte {
	b := make([]byte, len(samples)*2)
	for i, s := range samples {
		b[2*i] = byte(s)
		b[2*i+1] = byte(s >> 8)
	}
	return b
}
