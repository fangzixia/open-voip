package control

import (
	"context"
	"testing"

	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

func TestIVRFirstPromptWaitsForAnswer(t *testing.T) {
	svc, media, _, _ := newTestService("")
	media.holdAnswer = true
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1", SessionType: dto.SessionTypeAudio, SkipIVR: true})
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"start":"p","nodes":{"p":{"type":"play","file":"welcome.wav","next":"h"},"h":{"type":"hangup"}}}`
	before := media.injects
	if err := svc.startIVRPayload(ctx, id, ports.IVRSnapshot{FlowID: "f1", Version: 1, PayloadJSON: payload}); err != nil {
		t.Fatal(err)
	}
	if media.injects != before {
		t.Fatalf("prompt played before answer: before=%d after=%d", before, media.injects)
	}
	svc.tickIVR(ctx, id)
	if len(media.answerFns) != 1 {
		t.Fatalf("expected one deferred start, got %d", len(media.answerFns))
	}
	media.answerFns[0]()
	if media.injects != before+1 {
		t.Fatalf("prompt must play once after answer, before=%d got=%d", before, media.injects)
	}
}

func TestIVRPlayAdvancesOnPromptFinished(t *testing.T) {
	svc, _, _, _ := newTestService("")
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1", SessionType: dto.SessionTypeAudio, SkipIVR: true})
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"start":"p","nodes":{"p":{"type":"play","file":"welcome.wav","next":"h","timeout_sec":30},"h":{"type":"hangup"}}}`
	if err := svc.startIVRPayload(ctx, id, ports.IVRSnapshot{FlowID: "f1", Version: 1, PayloadJSON: payload}); err != nil {
		t.Fatal(err)
	}
	view, _ := svc.GetCall(ctx, id)
	if view.State != "ivr" {
		t.Fatalf("expected ivr, got %s", view.State)
	}
	svc.OnIVRPromptFinished(ctx, id, false)
	view, err = svc.GetCall(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != "ended" {
		t.Fatalf("play node should advance to hangup without waiting timeout, state=%s", view.State)
	}
}

func TestIVRInstantLoopHitsHopLimit(t *testing.T) {
	svc, _, _, _ := newTestService("")
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1", SessionType: dto.SessionTypeAudio})
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"start":"a","nodes":{"a":{"type":"tts","default":"b"},"b":{"type":"tts","default":"a"}}}`
	if err := svc.startIVRPayload(ctx, id, ports.IVRSnapshot{FlowID: "f1", Version: 1, PayloadJSON: payload}); err != nil {
		t.Fatal(err)
	}
	view, err := svc.GetCall(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != "ended" {
		t.Fatalf("instant IVR loop must end the call, state=%s", view.State)
	}
}
