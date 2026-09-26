// 本文件验证recovery的关键行为。
package control

import (
	"context"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"sync"
	"testing"
)

type cdrCapture struct {
	mu   sync.Mutex
	last ports.CDRWriteRequest
}

func (c *cdrCapture) Upsert(_ context.Context, v ports.CDRWriteRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last = v
	return nil
}

func (f *fakePersist) Unfinished(context.Context) ([]ports.CallRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []ports.CallRecord{}
	for _, rec := range f.calls {
		if rec.State != stateEnded {
			out = append(out, rec)
		}
	}
	return out, nil
}

func TestRestartClosesCallAndPreservesAnsweredAgent(t *testing.T) {
	svc, media, agents, _ := newTestService("ag1")
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1", Caller: "13800000000"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Answer(ctx, id, "ag1"); err != nil {
		t.Fatal(err)
	}
	cdr := &cdrCapture{}
	restarted := NewService(Deps{Media: media, Agents: agents, CDR: cdr, Calls: svc.deps.Calls})
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	view, err := restarted.GetCall(ctx, id)
	if err != nil || view.State != stateEnded {
		t.Fatalf("recovery: %+v %v", view, err)
	}
	if cdr.last.AgentID != "ag1" || cdr.last.Caller != "13800000000" || cdr.last.AnsweredAt == nil || cdr.last.EndedAt == nil {
		t.Fatalf("incomplete recovered CDR: %+v", cdr.last)
	}
	if agents.states["ag1"] != "acw" {
		t.Fatal("seat remained occupied")
	}
}

func TestConcurrentAnswerHangupCannotResurrectCall(t *testing.T) {
	for i := 0; i < 30; i++ {
		svc, _, _, _ := newTestService("ag1")
		ctx := context.Background()
		id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = svc.Answer(ctx, id, "ag1") }()
		go func() { defer wg.Done(); _ = svc.Hangup(ctx, id, dto.HangupReasonNormal) }()
		wg.Wait()
		view, err := svc.GetCall(ctx, id)
		if err != nil || view.State != stateEnded {
			t.Fatalf("call resurrected: %+v %v", view, err)
		}
	}
}
