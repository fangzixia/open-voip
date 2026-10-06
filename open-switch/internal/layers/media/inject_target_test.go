package media

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestPlayToneToRoomSingleLeg(t *testing.T) {
	callID := "c1"
	cap := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000}
	track, err := webrtc.NewTrackLocalStaticSample(cap, "a", "stream")
	if err != nil {
		t.Fatal(err)
	}
	r := &room{
		peers: map[string]*peer{
			"customer": {audioSamp: track},
			"agent":    {},
		},
		sipRTP: map[*sipRTP]struct{}{},
	}
	s := &Service{rooms: map[string]*room{callID: r}}
	seq := r.promptSeq.Add(1)
	s.playToneToRoom(callID, "customer", 25*time.Millisecond, seq, false)
	s.playToneToRoom(callID, "unknown", 25*time.Millisecond, seq, false)
}
