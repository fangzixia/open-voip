package media

import (
	"testing"
	"time"

	"open-switch/internal/ports/dto"
)

func TestGateInboundUntilPrompt(t *testing.T) {
	started := time.Now()
	rec := &recorder{
		started:                started,
		pcm:                    newPCMMix(8000, started),
		recordEngine:           "tap",
		gateInboundUntilPrompt: true,
	}
	in := mulawToneFrame()
	rec.TapUplink("customer", dto.LegRoleCustomer, pcmuPayloadToPCM(in), 8000)
	if len(rec.pcm.samples) > 0 {
		t.Fatal("inbound should be gated before prompt")
	}
	recordPromptToMix(rec, in)
	if len(rec.pcm.samples) < 160 {
		t.Fatal("prompt should be mixed")
	}
	rec.TapUplink("customer", dto.LegRoleCustomer, pcmuPayloadToPCM(in), 8000)
	if len(rec.pcm.samples) > 200 {
		t.Fatal("inbound should stay gated after prompt while IVR")
	}
	rec.gateInboundUntilPrompt = false
	rec.legPCM = map[string]*pcmMix{}
	rec.TapUplink("customer", dto.LegRoleCustomer, pcmuPayloadToPCM(in), 8000)
	leg := rec.legPCM["customer"]
	if leg == nil || len(leg.samples) < 160 {
		t.Fatal("inbound should append to leg track after gate disabled")
	}
}
