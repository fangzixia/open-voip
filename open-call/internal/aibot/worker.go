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

// Worker owns only robot seat business sessions.
type Worker struct {
	cfg          config.AibotConfig
	sig          *switchapi.Client
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
		sig:          sw,
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
		_, err := w.sig.CheckIn(ctx, w.agentID, queueIDs)
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

// OnCallEvent drives robot seat sessions independently of notifications.
func (w *Worker) OnCallEvent(ctx context.Context, ev ports.CallEvent) {
	if !w.cfg.Enabled {
		return
	}
	switch ev.Type {
	case "call.ringing":
		if ev.AgentID != w.agentID || w.agentID == "" || ev.CallID == "" {
			return
		}
		w.startInbound(ctx, ev.CallID)
	case "call.ended":
		w.mu.Lock()
		cancel := w.active[ev.CallID]
		w.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
}

// startInbound 为振铃呼入启动独立 goroutine 跑 RunInboundAI。
func (w *Worker) startInbound(parent context.Context, callID string) {
	w.mu.Lock()
	if _, ok := w.active[callID]; ok {
		w.mu.Unlock()
		return
	}
	sctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	w.active[callID] = cancel
	w.mu.Unlock()
	go func() {
		defer w.clearCall(callID)
		defer cancel()
		cfg := w.cfg
		if err := RunInboundAI(sctx, w.sig, cfg, callID, w.agentID, w.queuePrompts, w.usage, w.log); err != nil && w.log != nil {
			w.log.Warn("AI 呼入会话结束", "call_id", callID, "err", err)
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
