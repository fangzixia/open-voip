package aibot

import (
	"context"
	"errors"
	"log/slog"
	"open-call/internal/aibot/realtime"
	"open-call/internal/config"
	"open-call/internal/integration/switchapi"
	"open-call/internal/ports/dto"
	"sync/atomic"
	"time"
)

// RunInboundAI adapts model audio and conversation events to a generic PCM session.
// Switch owns seat reservation, codecs, pacing, buffering, mixing and cleanup.
func RunInboundAI(parent context.Context, sw *switchapi.Client, cfg config.AibotConfig, callID, agentID string, prompts QueuePromptStore, usage *UsageRecorder, log *slog.Logger) (result error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	defer func() {
		if result != nil {
			hc, stop := context.WithTimeout(context.Background(), 8*time.Second)
			defer stop()
			v, err := sw.GetCall(hc, callID)
			if err == nil && v.AgentID == agentID && v.State != "ended" && v.State != "transferring" {
				for _, l := range v.Legs {
					if l.Role == dto.LegRoleAgent && l.AgentID == agentID {
						_ = sw.Hangup(hc, callID, dto.HangupReasonError)
						break
					}
				}
			}
		}
	}()
	if err := sw.Answer(ctx, callID, agentID); err != nil {
		return err
	}
	view, err := sw.GetCall(ctx, callID)
	if err != nil {
		return err
	}
	legID := ""
	for _, l := range view.Legs {
		if l.Role == dto.LegRoleAgent && l.AgentID == agentID {
			legID = l.ID
			break
		}
	}
	if legID == "" {
		return errors.New("robot agent leg missing")
	}
	inRate, outRate := realtime.RealtimeInputRate(cfg.OpenAI), realtime.RealtimeOutputRate(cfg.OpenAI)
	stream, err := sw.OpenPCM(ctx, callID, legID, inRate, outRate, "duplex")
	if err != nil {
		return err
	}
	defer stream.Close()
	rs, err := realtime.Connect(ctx, cfg.OpenAI, resolveSystemPrompt(ctx, cfg, prompts, view.QueueID))
	if err != nil {
		return err
	}
	defer rs.Close()
	rs.BindCallLog(log, callID)
	var inputMs, outputMs atomic.Int64
	start := time.Now()
	defer func() {
		if usage != nil {
			usage.RecordAISession(callID, agentID, view.QueueID, start, inputMs.Load(), outputMs.Load())
		}
	}()
	rs.Start(ctx, realtime.OutputCallbacks{
		Audio: func(g uint64, pcm []byte) error {
			outputMs.Add(int64(len(pcm)/2) * 1000 / int64(outRate))
			return stream.Send(ctx, g, pcm)
		},
		Clear:  func(g uint64) error { return stream.Clear(ctx, g) },
		Finish: func(g uint64) error { return stream.Finish(ctx, g) },
	})
	inputErr := make(chan error, 1)
	go func() {
		for {
			pcm, err := stream.Receive(ctx)
			if err != nil {
				inputErr <- err
				return
			}
			inputMs.Add(int64(len(pcm)/2) * 1000 / int64(inRate))
			if err = rs.AppendInputPCM(ctx, pcm); err != nil {
				inputErr <- err
				return
			}
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-rs.Done():
		return err
	case err := <-inputErr:
		return err
	}
}
