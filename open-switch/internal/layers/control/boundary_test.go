package control

import (
	"context"
	"testing"
	"time"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"
)

type entryConfig struct {
	fakeCfg
	snapshot ports.IVRSnapshot
	err      error
	queue    ports.QueueSnapshot
}

func (c entryConfig) GetQueue(ctx context.Context, id string) (ports.QueueSnapshot, error) {
	if c.queue.ID != "" {
		return c.queue, nil
	}
	return c.fakeCfg.GetQueue(ctx, id)
}

func (c entryConfig) GetLatestIVR(context.Context, string) (ports.IVRSnapshot, error) {
	return c.snapshot, c.err
}

func TestOutboundConflictPreservesExistingCall(t *testing.T) {
	for _, state := range []string{stateRinging, stateActive, stateHeld} {
		t.Run(state, func(t *testing.T) {
			svc, media, agents, _ := newTestService("ag1")
			ctx := context.Background()
			id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
			if err != nil {
				t.Fatal(err)
			}
			if state != stateRinging {
				if err := svc.Answer(ctx, id, "ag1"); err != nil {
					t.Fatal(err)
				}
			}
			if state == stateHeld {
				if err := svc.Hold(ctx, id, true); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := svc.GetCall(ctx, id)
			agentBefore, _ := agents.ByID(ctx, "ag1")
			_, err = svc.Outbound(ctx, dto.OutboundRequest{AgentID: "ag1", Destination: "+8613800138000"})
			if api := errs.AsAPIError(err); api == nil || api.Code != errs.CodeAgentBusy {
				t.Fatalf("expected agent busy, got %v", err)
			}
			after, _ := svc.GetCall(ctx, id)
			agentAfter, _ := agents.ByID(ctx, "ag1")
			if after.State != before.State || after.Version != before.Version || agentAfter.State != agentBefore.State || media.closed {
				t.Fatalf("existing call was changed: before=%+v after=%+v", before, after)
			}
			persist := svc.deps.Calls.(*fakePersist)
			if len(persist.calls) != 1 {
				t.Fatalf("rejected outbound persisted a call: %d", len(persist.calls))
			}
			if err := svc.Hangup(ctx, id, dto.HangupReasonNormal); err != nil {
				t.Fatal(err)
			}
			media.sipOK = true
			if _, err := svc.Outbound(ctx, dto.OutboundRequest{AgentID: "ag1", Destination: "+8613800138000"}); err != nil {
				t.Fatalf("explicit hangup must allow new outbound: %v", err)
			}
		})
	}
}

func TestCollectInputEmitsRawInputAndTimesOut(t *testing.T) {
	for _, digit := range []string{"0", "9", "*", "#", ""} {
		t.Run(digit, func(t *testing.T) {
			svc, _, _, events := newTestService("")
			ctx := context.Background()
			id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1", SkipIVR: true})
			if err != nil {
				t.Fatal(err)
			}
			payload := `{"start":"input","nodes":{"input":{"type":"collect_input","accepted_digits":"09*#","result_key":"reference","timeout_sec":2,"next":"end","default":"end"},"end":{"type":"hangup"}}}`
			if err := svc.startIVRPayload(ctx, id, ports.IVRSnapshot{FlowID: "flow", Version: 7, PayloadJSON: payload}); err != nil {
				t.Fatal(err)
			}
			svc.onDTMF(ctx, id, "5")
			if len(events.payloads["ivr.input_collected"]) != 0 {
				t.Fatal("invalid digit emitted a result")
			}
			if digit == "" {
				svc.calls[id].ivr.entered = time.Now().Add(-time.Minute)
				svc.tickIVR(ctx, id)
			} else {
				svc.onDTMF(ctx, id, digit)
				svc.onDTMF(ctx, id, digit)
			}
			view, _ := svc.GetCall(ctx, id)
			if view.State != stateEnded {
				t.Fatalf("state=%s", view.State)
			}
			results := events.payloads["ivr.input_collected"]
			if digit == "" {
				if len(results) != 0 {
					t.Fatal("timeout emitted input")
				}
			} else {
				if len(results) != 1 || results[0]["input"] != digit || results[0]["result_key"] != "reference" || results[0]["node_id"] != "input" || results[0]["flow_version"] != 7 {
					t.Fatalf("raw results: %+v", results)
				}
				if _, ok := results[0]["score"]; ok {
					t.Fatal("switch interpreted input as score")
				}
			}
		})
	}
}

func TestEnterIVRDetachesAgentAndRejectsMissingFlowBeforeMutation(t *testing.T) {
	ctx := context.Background()
	svc, _, agents, _ := newTestService("ag1")
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Answer(ctx, id, "ag1"); err != nil {
		t.Fatal(err)
	}
	before, _ := svc.GetCall(ctx, id)
	svc.deps.Config = entryConfig{err: errs.NotFound("missing flow")}
	if err := svc.EnterIVR(ctx, id, "missing"); err == nil {
		t.Fatal("missing flow accepted")
	}
	after, _ := svc.GetCall(ctx, id)
	if after.State != before.State || len(after.Legs) != len(before.Legs) {
		t.Fatal("missing flow changed call")
	}
	svc.deps.Config = entryConfig{snapshot: ports.IVRSnapshot{PayloadJSON: `{"start":"old","nodes":{"old":{"type":"csat"}}}`}}
	if err := svc.EnterIVR(ctx, id, "old"); err == nil {
		t.Fatal("unsupported snapshot accepted")
	}
	after, _ = svc.GetCall(ctx, id)
	if after.State != before.State || len(after.Legs) != len(before.Legs) {
		t.Fatal("invalid snapshot changed call")
	}
	if err := svc.Hold(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	svc.deps.Config = entryConfig{snapshot: ports.IVRSnapshot{FlowID: "flow", Version: 1, PayloadJSON: `{"start":"input","nodes":{"input":{"type":"collect_input","accepted_digits":"0123456789","result_key":"reference","next":"end","default":"end"},"end":{"type":"hangup"}}}`}}
	if err := svc.EnterIVR(ctx, id, "flow"); err != nil {
		t.Fatal(err)
	}
	view, _ := svc.GetCall(ctx, id)
	if view.State != stateIVR || view.Held {
		t.Fatalf("state=%s", view.State)
	}
	for _, leg := range view.Legs {
		if leg.Role == dto.LegRoleAgent {
			t.Fatal("agent leg remained live")
		}
	}
	legs, _ := svc.deps.Calls.ListLegs(ctx, id)
	for _, leg := range legs {
		if leg.Role == dto.LegRoleAgent {
			t.Fatal("agent leg remained persisted")
		}
	}
	agent, _ := agents.ByID(ctx, "ag1")
	if agent.State != "acw" || len(svc.openCallsForAgent(ctx, "ag1")) != 0 {
		t.Fatalf("agent still occupied: %+v", agent)
	}
}

func TestEnterIVRImmediateNodeAndReplay(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newTestService("ag1")
	svc.deps.Commands = &memCommands{}
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Answer(ctx, id, "ag1"); err != nil {
		t.Fatal(err)
	}
	view, _ := svc.GetCall(ctx, id)
	svc.deps.Config = entryConfig{snapshot: ports.IVRSnapshot{FlowID: "end-flow", Version: 1, PayloadJSON: `{"start":"end","nodes":{"end":{"type":"hangup"}}}`}}
	commandCtx := scope.WithExpectedVersion(scope.WithIdempotency(ctx, "enter-ivr"), view.Version)
	if err := svc.EnterIVR(commandCtx, id, "end-flow"); err != nil {
		t.Fatal(err)
	}
	ended, _ := svc.GetCall(ctx, id)
	if ended.State != stateEnded {
		t.Fatalf("immediate node did not finish: %+v", ended)
	}
	if err := svc.EnterIVR(commandCtx, id, "end-flow"); err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	replayed, _ := svc.GetCall(ctx, id)
	if replayed.Version != ended.Version {
		t.Fatal("replay executed twice")
	}
}

func TestFollowupFlowIsPinnedWhenQueueConfigurationChanges(t *testing.T) {
	svc, _, _, _ := newTestService("")
	svc.deps.Config = entryConfig{queue: ports.QueueSnapshot{ConfigVersion: 1, ID: "q1", Name: "Support", MaxWaitSec: 300, PostCallIVRFlowID: "original"}}
	id, err := svc.StartInbound(context.Background(), dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	svc.deps.Config = entryConfig{queue: ports.QueueSnapshot{ConfigVersion: 2, ID: "q1", PostCallIVRFlowID: "replacement"}}
	view, _ := svc.GetCall(context.Background(), id)
	if view.PostCallIVRFlowID != "original" {
		t.Fatalf("flow changed with active config: %+v", view)
	}
}
