package aibot

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"open-call/internal/config"
	"open-call/internal/errs"
	"open-call/internal/integration/switchapi"
	"open-call/internal/ports"
)

// Worker 虚拟坐席：AI 呼入与语音通知媒体。
type Worker struct {
	cfg          config.AibotConfig
	sig          *Signaling
	usage        *UsageRecorder
	log          *slog.Logger
	agentID      string
	queuePrompts QueuePromptStore

	mu     sync.Mutex
	active map[string]context.CancelFunc
}

// NewWorker 构造 Worker。
func NewWorker(cfg config.AibotConfig, sw *switchapi.Client, usage *UsageRecorder, queuePrompts QueuePromptStore, log *slog.Logger) *Worker {
	return &Worker{
		cfg:          cfg,
		sig:          NewSignaling(sw),
		usage:        usage,
		log:          log,
		agentID:      cfg.AgentID,
		queuePrompts: queuePrompts,
		active:       make(map[string]context.CancelFunc),
	}
}

const (
	checkInBackoffMin   = 500 * time.Millisecond
	checkInBackoffMax   = 5 * time.Second
	checkInSteadyPeriod = 30 * time.Second
)

// Run 登录、签入并阻塞至 ctx 结束。
func (w *Worker) Run(ctx context.Context) error {
	if !w.cfg.Enabled {
		return nil
	}
	if len(w.cfg.QueueIDs) > 0 {
		go w.maintainCheckIn(ctx)
	}
	<-ctx.Done()
	w.stopAll()
	return ctx.Err()
}

// maintainCheckIn 在 Switch 未就绪时退避重试签入，成功后定期重新签入以从 ACW/离线恢复。
func (w *Worker) maintainCheckIn(ctx context.Context) {
	queueIDs := w.cfg.QueueIDs
	backoff := checkInBackoffMin
	checkedIn := false
	for {
		err := w.sig.api.CheckIn(ctx, w.agentID, queueIDs)
		wait := checkInSteadyPeriod
		if err != nil {
			if w.log != nil && !isBusyCheckIn(err) {
				w.log.Warn("AI 坐席签入失败", "err", err)
			}
			checkedIn = false
			wait = backoff
			if backoff < checkInBackoffMax {
				backoff *= 2
				if backoff > checkInBackoffMax {
					backoff = checkInBackoffMax
				}
			}
		} else {
			if !checkedIn && w.log != nil {
				w.log.Info("AI 坐席已签入", "agent_id", w.agentID, "queue_ids", queueIDs)
			}
			checkedIn = true
			backoff = checkInBackoffMin
		}
		if !waitCtx(ctx, wait) {
			return
		}
	}
}

func isBusyCheckIn(err error) bool {
	var api *errs.APIError
	if errors.As(err, &api) {
		return api.Code == errs.CodeAgentBusy
	}
	return false
}

func waitCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// OnCallEvent 由 WS Hub 回调，驱动自动接听与语音通知。
//
// call.ringing：ACD 分给本 agent 时自动 RunInboundAI（与人工点应答等价，但由 Worker 代答）。
// call.outbound_progress / call.answered：外呼语音通知在媒体就绪后播放素材 WAV。
// active 表保证每 call_id 仅一个会话 goroutine。
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

// startInbound 为振铃呼入启动独立 goroutine 跑 RunInboundAI。
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
		if err := RunInboundAI(sctx, w.sig, cfg, callID, w.agentID, w.queuePrompts, w.usage, w.log); err != nil && w.log != nil {
			w.log.Warn("AI 呼入会话结束", "call_id", callID, "err", err)
		}
	}()
}

// maybePrompt 对外呼语音通知在媒体就绪后播放 IVR 素材。
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
	sctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	w.active[callID] = cancel
	w.mu.Unlock()
	go func() {
		defer func() {
			cancel()
			w.clearCall(callID)
		}()
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
