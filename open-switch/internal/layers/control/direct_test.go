package control

import (
	"context"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"testing"
)

func TestDirectCallWithoutBusinessPlatform(t *testing.T) {
	ctx := context.Background()
	events := &fakeEvents{}
	media := &fakeMedia{}
	svc := NewService(Deps{Media: media, Calls: newFakePersist(), CDR: &fakeCDR{}, CallEvents: events})
	request := dto.DirectCallRequest{Direction: "inbound", Caller: "1000", Callee: "2000", SessionType: dto.SessionTypeAudio}
	first, err := svc.CreateDirect(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != stateCreated || len(first.Legs) != 1 || first.Legs[0].Role != dto.LegRoleCustomer {
		t.Fatalf("unexpected direct call: %+v", first)
	}
	request.CallID = first.ID
	repeated, err := svc.CreateDirect(ctx, request)
	if err != nil || repeated.ID != first.ID || len(repeated.Legs) != 1 {
		t.Fatalf("create must be idempotent: %+v %v", repeated, err)
	}
	changed := request
	changed.InitialLegRole = dto.LegRoleAgent
	if _, err := svc.CreateDirect(ctx, changed); err == nil {
		t.Fatal("same call_id with another initial leg role must conflict")
	}
	withLeg, err := svc.AddDirectLeg(ctx, first.ID, dto.DirectLegRequest{Role: dto.LegRoleAgent})
	if err != nil || len(withLeg.Legs) != 2 {
		t.Fatalf("add leg: %+v %v", withLeg, err)
	}
	if _, err := svc.AddDirectLeg(ctx, first.ID, dto.DirectLegRequest{Role: dto.LegRoleAgent}); err == nil {
		t.Fatal("third leg must be rejected")
	}
	if err := svc.BridgeDirect(ctx, first.ID, first.Legs[0].ID, withLeg.Legs[1].ID); err != nil {
		t.Fatal(err)
	}
	active, err := svc.GetCall(ctx, first.ID)
	if err != nil || active.State != stateActive {
		t.Fatalf("bridge: %+v %v", active, err)
	}
	if err := svc.Hangup(ctx, first.ID, dto.HangupReasonNormal); err != nil {
		t.Fatal(err)
	}
	ended, err := svc.GetCall(ctx, first.ID)
	if err != nil || ended.State != stateEnded {
		t.Fatalf("hangup: %+v %v", ended, err)
	}
	if len(events.types) < 3 {
		t.Fatalf("missing call events: %v", events.types)
	}
	var _ ports.DirectControlPort = svc
}
