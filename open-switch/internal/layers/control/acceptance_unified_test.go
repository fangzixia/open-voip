package control

import (
	"context"
	"sync"
	"testing"
	"time"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"
)

type memCommands struct {
	mu    sync.Mutex
	byKey map[string]struct {
		view ports.CommandView
		hash string
	}
}

func (m *memCommands) Accept(ctx context.Context, callID, idempotencyKey, requestHash, typ string, result map[string]any) (ports.CommandView, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byKey == nil {
		m.byKey = map[string]struct {
			view ports.CommandView
			hash string
		}{}
	}
	app := scope.Application(ctx)
	key := app + "|" + idempotencyKey
	if existing, ok := m.byKey[key]; ok {
		if existing.hash != requestHash {
			return ports.CommandView{}, false, errs.Conflict("Idempotency-Key 已用于不同请求体", "")
		}
		return existing.view, true, nil
	}
	id := "cmd-" + idempotencyKey
	view := ports.CommandView{
		ID: id, ApplicationID: app, CallID: callID, Type: typ, Status: "accepted",
		IdempotencyKey: idempotencyKey, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	m.byKey[key] = struct {
		view ports.CommandView
		hash string
	}{view: view, hash: requestHash}
	return view, false, nil
}

func (m *memCommands) MarkRunning(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.byKey {
		if v.view.ID == id {
			v.view.Status = "running"
			m.byKey[k] = v
			return nil
		}
	}
	return nil
}

func (m *memCommands) Complete(ctx context.Context, id, status, errCode string, result map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.byKey {
		if v.view.ID == id {
			v.view.Status = status
			v.view.ErrorCode = errCode
			m.byKey[k] = v
			return nil
		}
	}
	return nil
}

func (m *memCommands) Get(ctx context.Context, id string) (ports.CommandView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.byKey {
		if v.view.ID == id {
			return v.view, nil
		}
	}
	return ports.CommandView{}, errs.NotFound("命令不存在")
}

func (m *memCommands) ReconcileStale(context.Context, time.Duration) error { return nil }

// §9 验收骨架：接听命令在相同 Idempotency-Key 下只执行一次（无需 PostgreSQL）。
func TestSection9AnswerIdempotency(t *testing.T) {
	ctx := scope.WithApplication(context.Background(), "test")
	svc, _, _, _ := newTestService("ag1")
	svc.deps.Commands = &memCommands{}
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	answerCtx := scope.WithApplication(scope.WithIdempotency(ctx, "answer-once"), "test")
	if err := svc.Answer(answerCtx, id, "ag1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Answer(answerCtx, id, "ag1"); err != nil {
		t.Fatal("idempotent replay:", err)
	}
	view, err := svc.GetCall(ctx, id)
	if err != nil || view.State != stateActive {
		t.Fatalf("call %+v err=%v", view, err)
	}
}
