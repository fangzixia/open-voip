package media

import (
	"net"
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
	socket := udpSocket(t)
	rt := &sipRTP{legID: "cust", conn: socket, remote: socket.LocalAddr().(*net.UDPAddr)}
	r := &room{
		peers:           map[string]*peer{},
		legRoles:        map[string]dto.LegRole{},
		promptFadeTotal: 3,
		sipRTP:          map[*sipRTP]struct{}{rt: {}},
	}
	seq := r.promptSeq.Add(1)
	r.promptStopAt = time.Now().Add(-time.Millisecond)
	pcm := make([]int16, 8000)
	for i := range pcm {
		pcm[i] = 9000
	}
	r.prompt = &roomPrompt{target: "cust", generation: seq, pcm: pcm, loop: true, readySince: time.Now().Add(-time.Second)}
	s := &Service{}
	for i, expected := range []int16{9000, 6000, 3000} {
		frame, target := s.promptFrameLocked("call", r, time.Now())
		if len(frame) != 160 || target != "cust" || frame[159] < expected-1 || frame[159] > expected {
			t.Fatalf("fade did not use room PCM frame: frame=%d expected=%d", i, expected)
		}
		if i < 2 && r.promptSeq.Load() != seq {
			t.Fatal("generation advanced before fade completed")
		}
	}
	if r.promptSeq.Load() == seq {
		t.Fatal("seq should advance after fade")
	}
	if frame, _ := s.promptFrameLocked("call", r, time.Now()); len(frame) != 0 {
		t.Fatal("cancelled prompt generation resumed")
	}
}
