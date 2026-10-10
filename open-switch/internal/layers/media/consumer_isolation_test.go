package media

import (
	"context"
	"net"
	"testing"
	"time"

	"open-switch/internal/ports"
)

func TestBlockedLoggingAndApplicationDoNotStopRoomClock(t *testing.T) {
	s := &Service{rooms: map[string]*room{}}
	entered, release := make(chan struct{}), make(chan struct{})
	s.enqueueMediaObservation(mediaQualityEvent{emit: func() { close(entered); <-release }})
	defer close(release)
	<-entered
	start := time.Now()
	for range 1000 {
		s.enqueueMediaObservation(mediaQualityEvent{emit: func() {}})
	}
	if time.Since(start) > 50*time.Millisecond || len(s.qualityQueue) != 32 {
		t.Fatal("blocked log consumer stalled transport callbacks or queue was unbounded")
	}
	receiver, sender := udpSocket(t), udpSocket(t)
	rt := &sipRTP{legID: "customer", conn: sender, remote: receiver.LocalAddr().(*net.UDPAddr)}
	m := newScheduledRoomMixer()
	r := &room{mixAudio: true, mixer: m, peers: map[string]*peer{}, sipRTP: map[*sipRTP]struct{}{rt: {}}}
	s.rooms["isolated"] = r
	a, err := s.OpenApplicationStream(context.Background(), "isolated", "agent", ports.MediaStreamOptions{
		Direction: "duplex", Input: ports.PCMFormat{SampleRate: 8000}, Output: ports.PCMFormat{SampleRate: 8000},
	})
	if err != nil {
		t.Fatal(err)
	}
	go s.runRoomMixLoop("isolated", r, m)
	defer m.stop()
	defer a.Close()
	_ = receiver.SetReadDeadline(time.Now().Add(4 * time.Second))
	buf := make([]byte, 1500)
	// Do not consume application input. Its two-second queue must fail, while
	// the room continues to send the following forty frames to the phone.
	for range 140 {
		if _, _, err = receiver.ReadFromUDP(buf); err != nil {
			t.Fatal("room stopped while non-media consumers were blocked:", err)
		}
	}
	select {
	case <-a.Done():
	default:
		t.Fatal("stalled application was not closed at its bounded input limit")
	}
	if q := m.quality(nil); q.OutputDropped != 0 {
		t.Fatal("unrelated phone output overflowed")
	}
}
