package cccore

import (
	"context"
	"github.com/google/uuid"
	"gorm.io/gorm/logger"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"
	"open-switch/internal/store"
	"open-switch/internal/store/migrate"
	"os"
	"sync"
	"testing"
	"time"
)

// TestConfigurationIsolationAndConcurrentDispatch 验证多应用配置隔离与并发派单互不串扰。
func TestConfigurationIsolationAndConcurrentDispatch(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	db, err := store.Open(dsn, logger.Silent)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Migrate(db); err != nil {
		t.Fatal(err)
	}
	events := store.CallEvents{DB: db}
	svc := New(db, events, Options{})
	ctx := context.Background()
	q, a, b, flow := uuid.New().String(), uuid.New().String(), uuid.New().String(), uuid.New().String()
	bundle := ports.ConfigBundle{
		Queues: []ports.QueueConfig{{ID: q, Name: "voice", AgentIDs: []string{a, b}}},
		Agents: []ports.AgentConfig{{ID: a, UserRef: "u1", Extension: "1001", Enabled: true}, {ID: b, UserRef: "u2", Extension: "1002", Enabled: false}},
		DIDs:   []ports.DIDConfig{{ID: uuid.New().String(), TrunkID: "*", DID: "8001", TargetType: "queue", TargetID: q}},
		IVRs:   []ports.IVRConfig{{FlowID: flow, Version: 1, PayloadJSON: `{"start":"end","nodes":{"end":{"type":"hangup"}}}`}},
	}
	v, err := svc.StoreConfig(ctx, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetQueue(ctx, q); err == nil {
		t.Fatal("draft became active")
	}
	if _, err := svc.ActivateConfig(ctx, v.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetLatestIVR(ctx, flow); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ByID(ctx, b); err == nil {
		t.Fatal("disabled agent enabled by persistence")
	}
	if _, err := svc.CheckIn(ctx, a, []string{q}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetQueue(ctx, q); err != nil {
		t.Fatal("active queue missing:", err)
	}
	route, err := svc.ResolveDID(context.Background(), "*", "8001")
	if err != nil || route.ConfigVersion != v.Version {
		t.Fatalf("route %+v %v", route, err)
	}
	var wg sync.WaitGroup
	results := make(chan dto.DispatchResult, 8)
	errors := make(chan error, 8)
	callID := uuid.New().String()
	now := time.Now().UTC()
	if err := store.NewCallStore(db, events).InsertCall(ctx, ports.CallRecord{ID: callID, ConfigVersion: &v.Version, Direction: "inbound", State: "queued", QueueID: &q, SessionType: dto.SessionTypeAudio, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := svc.RequestAgent(ctx, dto.DispatchRequest{CallID: callID, QueueID: q})
			results <- r
			errors <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for e := range errors {
		if e != nil {
			t.Fatal(e)
		}
	}
	for r := range results {
		if r.AgentID != a {
			t.Fatalf("dispatch=%+v", r)
		}
	}
	other, err := svc.RequestAgent(ctx, dto.DispatchRequest{CallID: uuid.New().String(), QueueID: q})
	if err != nil || other.AgentID != "" {
		t.Fatalf("agent reserved twice: %+v %v", other, err)
	}
	if err := svc.SetCallState(ctx, callID, a, "ringing", "on_call", "answer"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetCallState(ctx, callID, a, "on_call", "acw", "hangup"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPresence(ctx, a, "idle", "wrap-up"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckOut(ctx, a); err != nil {
		t.Fatal(err)
	}
	rows, err := events.List(ctx, 0, "", 100)
	if err != nil || len(rows) == 0 {
		t.Fatalf("no durable agent events %v", err)
	}
	bundle.Queues[0].Name = "new"
	next, err := svc.StoreConfig(ctx, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ActivateConfig(ctx, next.Version); err != nil {
		t.Fatal(err)
	}
	pinned, err := svc.GetQueue(scope.WithConfigVersion(ctx, v.Version), q)
	if err != nil || pinned.Name != "voice" {
		t.Fatalf("old call lost pinned config %+v %v", pinned, err)
	}
}
