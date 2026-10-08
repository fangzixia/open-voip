package control

import (
	"context"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"
	"testing"
	"time"
)

type playbackMedia struct {
	fakeMedia
	starts  int
	stopped string
	done    func(string)
}

func (m *playbackMedia) OpenApplicationStream(context.Context, string, string, ports.MediaStreamOptions) (ports.ApplicationStream, error) {
	return nil, nil
}
func (m *playbackMedia) StartPlayback(_ context.Context, _, _, _, _ string, done func(string)) error {
	m.starts++
	m.done = done
	return nil
}
func (m *playbackMedia) StopPlayback(_ context.Context, _, _, id string) error {
	m.stopped = id
	return nil
}

func TestNativePlaybackIdempotencyAndScope(t *testing.T) {
	ctx := context.Background()
	m := &playbackMedia{}
	s := NewService(Deps{Media: m, Calls: newFakePersist(), CDR: &fakeCDR{}})
	v, err := s.CreateDirect(ctx, dto.DirectCallRequest{SessionType: dto.SessionTypeAudio})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.transition(ctx, v.ID, stateActive); err != nil {
		t.Fatal(err)
	}
	cmd := scope.WithIdempotency(ctx, "play-once")
	id, err := s.StartLegPlayback(cmd, v.ID, v.Legs[0].ID, "asset-a")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.StartLegPlayback(cmd, v.ID, v.Legs[0].ID, "asset-a")
	if err != nil || id2 != id || m.starts != 1 {
		t.Fatalf("replayed playback: %s %s %d %v", id, id2, m.starts, err)
	}
	if _, err = s.StartLegPlayback(cmd, v.ID, v.Legs[0].ID, "asset-b"); err == nil {
		t.Fatal("changed asset under same key accepted")
	}
	if err = s.StopLegPlayback(ctx, v.ID, "wrong-leg", id); err == nil {
		t.Fatal("wrong-leg stop accepted")
	}
	if err = s.StopLegPlayback(ctx, v.ID, v.Legs[0].ID, id); err != nil {
		t.Fatal(err)
	}
	if m.stopped != id || m.stopInjected != 0 {
		t.Fatal("stop escaped playback scope")
	}
	m.done("finished")
	p, err := s.GetLegPlayback(ctx, v.ID, v.Legs[0].ID, id)
	if err != nil || p.State != "finished" {
		t.Fatalf("missing completion: %+v %v", p, err)
	}
}

func TestGenericSIPOriginateHasNoSeatAndReplaysOnce(t *testing.T) {
	ctx := context.Background()
	m := &fakeMedia{sipOK: true}
	s := NewService(Deps{Media: m, Calls: newFakePersist(), CDR: &fakeCDR{}})
	v, err := s.CreateStubCall(ctx, dto.StubCallRequest{BusinessRef: "notification-business-task"})
	if err != nil {
		t.Fatal(err)
	}
	cmd := scope.WithIdempotency(ctx, "dial-once")
	req := dto.DirectSIPRequest{Destination: "+8613800138000"}
	r, err := s.DialDirectSIP(cmd, v.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := s.DialDirectSIP(cmd, v.ID, req)
	if err != nil || r2.LegID != r.LegID {
		t.Fatalf("dial replay: %+v %v", r2, err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		view, err := s.GetCall(ctx, v.ID)
		if err != nil {
			t.Fatal(err)
		}
		if view.State == stateActive {
			if view.AgentID != "" || len(view.Legs) != 1 || view.Legs[0].Role != dto.LegRolePSTN {
				t.Fatalf("originate occupied a seat: %+v", view)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("single-leg call never became active: %+v", view)
		}
		time.Sleep(time.Millisecond)
	}
	req.Destination = "+8613900139000"
	if _, err = s.DialDirectSIP(cmd, v.ID, req); err == nil {
		t.Fatal("changed destination reused same key")
	}
}
