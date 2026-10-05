package media

import (
	"testing"
	"time"

	"open-switch/internal/ports/dto"
)

func TestMediaForwardAllowedBlocksAgentToCustomerDuringGrace(t *testing.T) {
	r := &room{
		peers:    map[string]*peer{"ag": {role: dto.LegRoleAgent}},
		legRoles: map[string]dto.LegRole{"cust": dto.LegRoleCustomer},
	}
	r.connectGraceUntil = time.Now().Add(500 * time.Millisecond)
	if r.mediaForwardAllowed("ag", "cust") {
		t.Fatal("expected agent→customer blocked during grace")
	}
	if !r.mediaForwardAllowed("cust", "ag") {
		t.Fatal("customer→agent should still forward during grace")
	}
	r.connectGraceUntil = time.Time{}
	if !r.mediaForwardAllowed("ag", "cust") {
		t.Fatal("expected forward after grace")
	}
}

func TestPromptHandoffKeepsSeqUntilFadeCompletes(t *testing.T) {
	r := &room{
		peers:           map[string]*peer{},
		legRoles:        map[string]dto.LegRole{},
		promptFadeTotal: 3,
	}
	seq := r.promptSeq.Add(1)
	r.promptStopAt = time.Now().Add(-time.Millisecond)
	gain, cont := r.promptGainAndContinue(seq)
	if gain != 32767 || !cont {
		t.Fatalf("first frame after stopAt: gain=%d cont=%v", gain, cont)
	}
	if r.promptSeq.Load() != seq {
		t.Fatal("seq should not advance before fade")
	}
	for i := 0; i < 2; i++ {
		_, cont = r.promptGainAndContinue(seq)
		if !cont {
			t.Fatalf("fade frame %d ended early", i)
		}
	}
	_, cont = r.promptGainAndContinue(seq)
	if cont {
		t.Fatal("expected last fade frame to end playback")
	}
	if r.promptSeq.Load() == seq {
		t.Fatal("seq should advance after fade")
	}
}

func TestScaleMulawFrameReducesLevel(t *testing.T) {
	in := pcmToG711([]int16{10000, -10000}, 0)
	out := scaleMulawFrame(in, 16384)
	pcm := pcmuPayloadToPCM(out)
	if pcm[0] == 10000 && pcm[1] == -10000 {
		t.Fatal("expected attenuated samples")
	}
}
