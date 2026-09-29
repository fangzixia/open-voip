package control

import (
	"context"
	"testing"

	"open-switch/internal/errs"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"
)

func TestGuardExpectedVersionRejectsStaleClient(t *testing.T) {
	svc, _, _, _ := newTestService("ag1")
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	if rt := svc.calls[id]; rt != nil {
		rt.rec.Version = 10
	}
	svc.mu.Unlock()
	stale := scope.WithExpectedVersion(ctx, 9)
	guardErr := svc.guardExpectedVersion(stale, id)
	if guardErr == nil {
		t.Fatal("expected version conflict")
	}
	api := errs.AsAPIError(guardErr)
	if api == nil || api.HTTP != 409 || api.Code != "VERSION_MISMATCH" {
		t.Fatalf("unexpected error: %+v", err)
	}
	if api.Data == nil {
		t.Fatal("expected latest view in data")
	}
}
