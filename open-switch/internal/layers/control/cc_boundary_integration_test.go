package control

import (
	"context"
	"gorm.io/gorm/logger"
	"open-switch/internal/datetime"
	"open-switch/internal/layers/cccore"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"
	"open-switch/internal/store"
	"open-switch/internal/store/migrate"
	"os"
	"testing"
	"time"
	"uuid"
)

// TestLocalCCLifecycleIntegration 覆盖 IVR 业务判断、完成/重复提交与超时等呼叫控制边界。
func TestLocalCCLifecycleIntegration(t *testing.T) {
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
	core := cccore.New(db, events, cccore.Options{})
	svc, _, _, _ := newTestService("")
	svc.deps.BusinessActions = core
	svc.deps.Config = core
	svc.deps.ACD = core
	svc.deps.Agents = core
	svc.deps.Calls = store.NewCallStore(db)
	svc.deps.CDR = core
	svc.deps.Recordings = core
	svc.deps.CallEvents = events
	svc.deps.RecordingPolicy = core
	app := "lifecycle-" + uuid.New().String()
	ctx := scope.WithApplication(context.Background(), app)
	q, a, flow := uuid.New().String(), uuid.New().String(), uuid.New().String()
	bundle := ports.ConfigBundle{
		Queues: []ports.QueueConfig{{ID: q, Name: "voice", AgentIDs: []string{a}}},
		Agents: []ports.AgentConfig{{ID: a, UserRef: "employee", Extension: "1001", Enabled: true}},
		IVRs:   []ports.IVRConfig{{FlowID: flow, Version: 1, PayloadJSON: `{"start":"business","nodes":{"business":{"type":"business_action","action":"customer.eligible","timeout_sec":5,"choices":{"yes":"queue"},"default":"end"},"queue":{"type":"route_queue","queue_id":"` + q + `"},"end":{"type":"hangup"}}}`}},
	}
	v, err := core.StoreConfig(ctx, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = core.ActivateConfig(ctx, v.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = core.CheckIn(ctx, a, []string{q}); err != nil {
		t.Fatal(err)
	}
	call, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: q, Caller: "customer"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.GetCall(ctx, call)
	if err != nil || view.State != stateRinging {
		t.Fatalf("%+v %v", view, err)
	}
	foreign := scope.WithApplication(context.Background(), "other")
	if _, err := svc.GetCall(foreign, call); err == nil {
		t.Fatal("cross-app call leaked")
	}
	if list, err := svc.ListCalls(foreign); err != nil || len(list) != 0 {
		t.Fatal("cross-app list leaked")
	}
	if err := svc.Answer(ctx, call, a); err != nil {
		t.Fatal(err)
	}
	if err := svc.Hangup(ctx, call, dto.HangupReasonNormal); err != nil {
		t.Fatal(err)
	}
	session, err := core.AgentSession(ctx, a)
	if err != nil || session.State != "acw" {
		t.Fatalf("session %+v %v", session, err)
	}
	rows, err := events.List(ctx, 0, call, 100)
	if err != nil {
		t.Fatal(err)
	}
	hasCDR := false
	for _, row := range rows {
		if row.Type == "cdr.updated" {
			hasCDR = true
			var req ports.CDRWriteRequest
			if err := datetime.UnmarshalCurrent(row.Payload, &req); err != nil {
				t.Fatalf("CDR event wire format: %v", err)
			}
		}
	}
	if !hasCDR {
		t.Fatal("missing technical CDR event")
	}
	if _, err := core.SetPresence(ctx, a, "busy", "break"); err != nil {
		t.Fatal(err)
	}
	ivrCall, err := svc.StartInbound(ctx, dto.InboundRequest{ApplicationID: app, ConfigVersion: v.Version, IVRFlowID: flow, Caller: "business"})
	if err != nil {
		t.Fatal(err)
	}
	actionID := svc.calls[ivrCall].ivr.actionID
	if actionID == "" {
		t.Fatal("business action not requested")
	}
	if err := svc.CompleteBusinessAction(ctx, ivrCall, actionID, "invalid"); err == nil {
		t.Fatal("undeclared business outcome accepted")
	}
	if err := svc.CompleteBusinessAction(ctx, ivrCall, actionID, "yes"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteBusinessAction(ctx, ivrCall, actionID, "yes"); err != nil {
		t.Fatalf("business replay: %v", err)
	}
	if err := svc.CompleteBusinessAction(ctx, ivrCall, actionID, "different"); err == nil {
		t.Fatal("conflicting action replay accepted")
	}
	status, err := core.QueueStatus(ctx, q)
	if err != nil || status.Waiting != 1 {
		t.Fatalf("queue %+v %v", status, err)
	}
	if err := svc.Hangup(ctx, ivrCall, dto.HangupReasonNormal); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteBusinessAction(ctx, ivrCall, actionID, "yes"); err != nil {
		t.Fatalf("completed action replay after hangup: %v", err)
	}
	timed, err := svc.StartInbound(ctx, dto.InboundRequest{ApplicationID: app, ConfigVersion: v.Version, IVRFlowID: flow})
	if err != nil {
		t.Fatal(err)
	}
	svc.calls[timed].ivr.entered = time.Now().Add(-time.Minute)
	svc.tickCall(ctx, timed, time.Now())
	view, err = svc.GetCall(ctx, timed)
	if err != nil || view.State != stateEnded {
		t.Fatalf("business timeout %+v %v", view, err)
	}
}
