package media

import (
	"context"
	"testing"

	"github.com/pion/webrtc/v4"

	"open-switch/internal/ports/dto"
)

func TestICECandidateBeforeAnswerIsQueued(t *testing.T) {
	local, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	remote, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	if _, err := local.CreateDataChannel("test", nil); err != nil {
		t.Fatal(err)
	}
	offer, err := local.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := local.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	if err := remote.SetRemoteDescription(offer); err != nil {
		t.Fatal(err)
	}
	answer, err := remote.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	p := &peer{pc: local}
	s := &Service{rooms: map[string]*room{"call": {peers: map[string]*peer{"leg": p}}}}
	if err := s.TrickleICE(context.Background(), "call", "leg", dto.ICECandidateInit{Candidate: ""}); err != nil {
		t.Fatalf("early candidate: %v", err)
	}
	if len(p.pendingICE) != 1 {
		t.Fatalf("queued candidates=%d", len(p.pendingICE))
	}
	if err := s.AcceptAnswer(context.Background(), "call", "leg", answer.SDP); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if len(p.pendingICE) != 0 {
		t.Fatalf("undrained candidates=%d", len(p.pendingICE))
	}
}
