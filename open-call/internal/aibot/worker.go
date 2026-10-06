package aibot

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"open-call/internal/config"
	"open-call/internal/integration/switchapi"
	"open-call/internal/ports"
)

// Worker 虚拟坐席：AI 呼入与语音通知媒体。
type Worker struct {
	cfg     config.AibotConfig
	sig     *Signaling
	usage   *UsageRecorder
	log     *slog.Logger
	agentID string

	mu       sync.Mutex
	active   map[string]context.CancelFunc
	queuePrompt map[string]string // reserved
}

// NewWorker 构造 Worker。
func NewWorker(cfg config.AibotConfig, sw *switchapi.Client, usage *UsageRecorder, log *slog.Logger) *Worker {
	return &Worker{
		cfg:    cfg,
		sig:    NewSignaling(sw),
		usage:  usage,
		log:    log,
		agentID: cfg.AgentID,
		active: make(map[string]context.CancelFunc),
	}
}

// Run 登录、签入并阻塞至 ctx 结束。
func (w *Worker) Run(ctx context.Context) error {
	if !w.cfg.Enabled {
		return nil
	}
	if len(w.cfg.QueueIDs) > 0 {
		if err := w.sig.api.CheckIn(ctx, w.agentID, w.cfg.QueueIDs); err != nil && w.log != nil {
			w.log.Warn("AI 坐席签入失败", "err", err)
		}
	}
	<-ctx.Done()
	w.stopAll()
	return ctx.Err()
}

// OnCallEvent 由 WS Hub 回调，驱动自动接听与语音通知。
func (w *Worker) OnCallEvent(ctx context.Context, ev ports.CallEvent) {
	if !w.cfg.Enabled {
		return
	}
	switch ev.Type {
	case "call.ringing":
		if ev.AgentID != "" && ev.AgentID != w.agentID {
			return
		}
		callID := ev.CallID
		if callID == "" {
			callID, _ = ev.Payload["call_id"].(string)
		}
		if callID == "" {
			return
		}
		w.startInbound(ctx, callID)
	case "call.outbound_progress":
		phase, _ := ev.Payload["phase"].(string)
		if phase != "media_ready" && phase != "connected" {
			return
		}
		callID := ev.CallID
		if callID == "" {
			callID, _ = ev.Payload["call_id"].(string)
		}
		w.maybePrompt(ctx, callID)
	case "call.answered":
		w.maybePrompt(ctx, ev.CallID)
	}
}

func (w *Worker) startInbound(parent context.Context, callID string) {
	w.mu.Lock()
	if _, ok := w.active[callID]; ok {
		w.mu.Unlock()
		return
	}
	sctx, cancel := context.WithCancel(parent)
	w.active[callID] = cancel
	w.mu.Unlock()
	go func() {
		defer w.clearCall(callID)
		cfg := w.cfg
		if err := RunInboundAI(sctx, w.sig, cfg, callID, w.agentID, w.usage, w.log); err != nil && w.log != nil {
			w.log.Warn("AI 呼入会话结束", "call_id", callID, "err", err)
		}
	}()
}

func (w *Worker) maybePrompt(ctx context.Context, callID string) {
	if callID == "" {
		return
	}
	view, err := w.sig.api.GetCall(ctx, callID)
	if err != nil {
		return
	}
	if view.OutboundMode != "prompt_outbound" {
		return
	}
	if view.PromptAssetID == "" {
		return
	}
	agentLeg, ok := findAgentLeg(view, w.agentID)
	if !ok {
		agentLeg, ok = findAgentLeg(view, view.AgentID)
	}
	if !ok {
		return
	}
	_ = agentLeg
	w.mu.Lock()
	if _, ok := w.active[callID]; ok {
		w.mu.Unlock()
		return
	}
	sctx, cancel := context.WithCancel(ctx)
	w.active[callID] = cancel
	w.mu.Unlock()
	go func() {
		defer w.clearCall(callID)
		time.Sleep(200 * time.Millisecond)
		if err := RunPromptOutbound(sctx, w.sig, callID, w.agentID, view.PromptAssetID, w.log); err != nil && w.log != nil {
			w.log.Warn("语音通知失败", "call_id", callID, "err", err)
		}
	}()
}

func (w *Worker) clearCall(callID string) {
	w.mu.Lock()
	delete(w.active, callID)
	w.mu.Unlock()
}

func (w *Worker) stopAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, cancel := range w.active {
		cancel()
		delete(w.active, id)
	}
}
