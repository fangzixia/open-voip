// 本文件验证 recovery 的关键行为。
package control

import (
	"context"
	"sync"
	"testing"
	"time"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
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

type memCallEvents struct {
	types []string
}

func (m *memCallEvents) PublishCallEvent(_ context.Context, ev ports.CallEvent) error {
	m.types = append(m.types, ev.Type)
	return nil
}

func (m *memCallEvents) hasType(want string) bool {
	for _, t := range m.types {
		if t == want {
			return true
		}
	}
	return false
}

func TestRecoverActiveCallHangups(t *testing.T) {
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
	events := &memCallEvents{}
	restarted := NewService(Deps{Media: media, Agents: agents, CDR: cdr, Calls: svc.deps.Calls, CallEvents: events})
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	view, err := restarted.GetCall(ctx, id)
	if err != nil || view.State != stateEnded {
		t.Fatalf("recovery should hangup active call: %+v %v", view, err)
	}
	if events.hasType("call.media_reconnect_required") {
		t.Fatal("recovery must not emit media reconnect")
	}
}

type recoveryIVRCfg struct {
	payload string
}

func (recoveryIVRCfg) ActiveVersion(context.Context) (int64, error) { return 1, nil }
func (c recoveryIVRCfg) GetQueue(_ context.Context, id string) (ports.QueueSnapshot, error) {
	return ports.QueueSnapshot{ConfigVersion: 1, ID: id, MaxWaitSec: 300}, nil
}
func (c recoveryIVRCfg) GetLatestIVR(context.Context, string) (ports.IVRSnapshot, error) {
	return ports.IVRSnapshot{}, nil
}
func (c recoveryIVRCfg) GetIVRSnapshot(_ context.Context, _ int64, flowID string, _ int) (ports.IVRSnapshot, error) {
	return ports.IVRSnapshot{FlowID: flowID, Version: 1, PayloadJSON: c.payload}, nil
}
func (c recoveryIVRCfg) GetBusinessHours(context.Context, string) (ports.BusinessHours, error) {
	return ports.BusinessHours{WeekdayHours: "always"}, nil
}
func (c recoveryIVRCfg) ResolveDID(context.Context, string, string) (ports.DIDRouteSnapshot, error) {
	return ports.DIDRouteSnapshot{}, nil
}
func (recoveryIVRCfg) Now(context.Context) time.Time { return time.Now().UTC() }
func (c recoveryIVRCfg) QueueStatus(_ context.Context, queueID string) (ports.QueueStatusView, error) {
	return ports.QueueStatusView{QueueID: queueID}, nil
}

type memIVRSessions struct {
	sess ports.IVRSessionView
}

func (m memIVRSessions) UpsertIVRSession(context.Context, string, string, int, string, string, *time.Time) error {
	return nil
}
func (m memIVRSessions) GetIVRSession(_ context.Context, callID string) (ports.IVRSessionView, error) {
	if m.sess.CallID == callID {
		return m.sess, nil
	}
	return ports.IVRSessionView{}, errs.NotFound("IVR 会话不存在")
}
func (m memIVRSessions) DeleteIVRSession(context.Context, string) error { return nil }

type memRouting struct {
	ports.RoutingSessionView
}

func (m memRouting) Get(_ context.Context, callID string) (ports.RoutingSessionView, error) {
	if m.CallID == callID {
		return m.RoutingSessionView, nil
	}
	return ports.RoutingSessionView{}, errs.NotFound("路由会话不存在")
}

func TestRecoverQueuedCallHangups(t *testing.T) {
	qID := "q1"
	cfgVer := int64(1)
	persist := newFakePersist()
	queuedAt := time.Now().UTC().Add(-2 * time.Minute)
	persist.calls["q-call"] = ports.CallRecord{
		ID: "q-call", State: stateQueued, Direction: "inbound",
		ConfigVersion: &cfgVer, Caller: "138", QueueID: &qID,
		CreatedAt: queuedAt, UpdatedAt: queuedAt,
	}
	persist.legs["q-call"] = []ports.CallLegRecord{{ID: "cust", CallID: "q-call", Role: dto.LegRoleCustomer, CreatedAt: queuedAt}}
	media := &fakeMedia{}
	svc := NewService(Deps{
		Media: media, ACD: &fakeACD{agent: "", agents: newFakeAgents()}, Config: recoveryIVRCfg{payload: `{}`}, Calls: persist,
		Routing: memRouting{RoutingSessionView: ports.RoutingSessionView{
			CallID: "q-call", QueueID: qID, EnqueuedAt: &queuedAt,
		}},
	})
	if err := svc.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	view, err := svc.GetCall(context.Background(), "q-call")
	if err != nil || view.State != stateEnded {
		t.Fatalf("expected queued call hung up, got %+v err=%v", view, err)
	}
}

func TestRecoverIVRCallHangups(t *testing.T) {
	flow := "f1"
	cfgVer := int64(1)
	payload := `{"start":"menu","nodes":{"menu":{"type":"menu","timeout_sec":30,"choices":{"1":"end"},"default":"end"},"end":{"type":"hangup"}}}`
	persist := newFakePersist()
	deadline := time.Now().UTC().Add(20 * time.Second)
	persist.calls["ivr-call"] = ports.CallRecord{
		ID: "ivr-call", State: stateIVR, Direction: "inbound",
		ConfigVersion: &cfgVer, Caller: "138", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	persist.legs["ivr-call"] = []ports.CallLegRecord{{ID: "cust", CallID: "ivr-call", Role: dto.LegRoleCustomer, CreatedAt: time.Now().UTC()}}
	media := &fakeMedia{}
	svc := NewService(Deps{
		Media: media, Config: recoveryIVRCfg{payload: payload}, Calls: persist,
		IVRSessions: memIVRSessions{sess: ports.IVRSessionView{
			CallID: "ivr-call", FlowID: flow, FlowVersion: 1,
			NodeID: "menu", StateJSON: `{}`, DeadlineAt: &deadline,
		}},
		BusinessActions: &fakeBusinessActions{},
	})
	if err := svc.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	view, err := svc.GetCall(context.Background(), "ivr-call")
	if err != nil || view.State != stateEnded {
		t.Fatalf("expected ivr call hung up, got %+v err=%v", view, err)
	}
}

type fakeBusinessActions struct{}

func (fakeBusinessActions) BeginBusinessAction(context.Context, ports.BusinessAction) error {
	return nil
}
func (fakeBusinessActions) GetBusinessAction(context.Context, string) (ports.BusinessAction, error) {
	return ports.BusinessAction{}, errs.NotFound("missing")
}
func (fakeBusinessActions) ResolveBusinessAction(context.Context, string, string) error { return nil }
func (fakeBusinessActions) ExpireBusinessAction(context.Context, string) error          { return nil }

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
