package media

import (
	"context"
	"errors"
	"github.com/pion/rtp"
	"net"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"testing"
	"time"
)

func TestApplicationOutputBoundGenerationAndTail(t *testing.T) {
	o := audioOutput{generation: 1}
	if err := o.write(1, make([]int16, maxOutputSamples)); err != nil {
		t.Fatal(err)
	}
	if err := o.write(1, []int16{1}); !errors.Is(err, ports.ErrAudioBackpressure) {
		t.Fatalf("unbounded output: %v", err)
	}
	if err := o.clear(2); err != nil {
		t.Fatal(err)
	}
	if err := o.write(1, []int16{42}); !errors.Is(err, ports.ErrStaleGeneration) {
		t.Fatalf("late canceled audio accepted: %v", err)
	}
	if err := o.writeRate(2, []int16{100}, 24000); err != nil {
		t.Fatal(err)
	}
	if err := o.writeRate(2, []int16{100, 100}, 24000); err != nil {
		t.Fatal(err)
	}
	if err := o.finish(2); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	frame, g, done := o.pull(now)
	if len(frame) != 160 || frame[0] != 100 || g != 2 || done {
		t.Fatalf("partial tail lost or completion too early: %v %d %v", frame, g, done)
	}
	if _, _, done = o.pull(now.Add(220 * time.Millisecond)); !done {
		t.Fatal("drained output never completed")
	}
	if _, _, done = o.pull(now.Add(time.Second)); done {
		t.Fatal("duplicate completion")
	}
}

func TestApplicationOutputUsesPhoneRTPClock(t *testing.T) {
	listen := func() *net.UDPConn {
		t.Helper()
		c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	receiver, sender := listen(), listen()
	rt := &sipRTP{legID: "customer", conn: sender, remote: receiver.LocalAddr().(*net.UDPAddr), remotePT: 8}
	r := &room{mixAudio: true, mixer: newScheduledRoomMixer(nil), peers: map[string]*peer{}, sipRTP: map[*sipRTP]struct{}{rt: {}}, streams: map[string]*applicationStream{}}
	s := &Service{rooms: map[string]*room{"call": r}}
	opts := ports.MediaStreamOptions{Direction: "duplex", Input: ports.PCMFormat{SampleRate: 16000}, Output: ports.PCMFormat{SampleRate: 24000}}
	a, err := s.OpenApplicationStream(context.Background(), "call", "agent", opts)
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, 960)
	for i := range pcm {
		pcm[i] = 4000
	}
	if err = a.Write(1, encodePCM16(pcm)); err != nil {
		t.Fatal(err)
	}
	if err = a.Finish(1); err != nil {
		t.Fatal(err)
	}
	var previous rtp.Packet
	for i := 0; i < 2; i++ {
		s.dispatchMixTick("call", r, r.mixer)
		_ = receiver.SetReadDeadline(time.Now().Add(time.Second))
		raw := make([]byte, 1500)
		n, _, err := receiver.ReadFromUDP(raw)
		if err != nil {
			t.Fatal(err)
		}
		var packet rtp.Packet
		if err = packet.Unmarshal(raw[:n]); err != nil {
			t.Fatal(err)
		}
		if packet.PayloadType != 8 || len(packet.Payload) != 160 {
			t.Fatalf("wrong negotiated G.711 format: %+v", packet)
		}
		if i > 0 && (packet.SequenceNumber != previous.SequenceNumber+1 || packet.Timestamp != previous.Timestamp+160) {
			t.Fatal("RTP continuity broken")
		}
		previous = packet
		if sample := pcmuPayloadToPCM(rtpPayloadToPCMU(8, packet.Payload))[0]; sample < 3000 {
			t.Fatalf("PCM did not reach phone: %d", sample)
		}
	}
	select {
	case <-a.Events():
		t.Fatal("completion preceded tail drain")
	default:
	}
}

func TestApplicationInputExcludesGeneratedAudioAndPlaybackIsolation(t *testing.T) {
	stream := &applicationStream{opts: ports.MediaStreamOptions{Direction: "duplex", Input: ports.PCMFormat{SampleRate: 16000}}, input: make(chan []byte, 2), events: make(chan ports.MediaOutputEvent, 2), done: make(chan struct{})}
	stream.output = audioOutput{generation: 1, pcm: []int16{2000}}
	stream.output.finished = true
	r := &room{streams: map[string]*applicationStream{"agent": stream}, playbacks: map[string]*assetPlayback{
		"a": {legID: "customer", output: audioOutput{pcm: make([]int16, 160), generation: 1, finished: true}},
		"b": {legID: "other", output: audioOutput{pcm: make([]int16, 160), generation: 1, finished: true}},
	}}
	frames := map[string][]int16{"customer": make([]int16, 160)}
	targets := r.applicationFrames(frames, time.Now())
	if len(<-stream.input) != 640 {
		t.Fatal("input not converted to requested 16k PCM16")
	}
	if frames["agent"][0] != 2000 || len(targets) != 2 {
		t.Fatal("output did not enter mixer or playback targets merged")
	}
	s := &Service{rooms: map[string]*room{"call": r}}
	if err := s.StopPlayback(context.Background(), "call", "customer", "b"); err == nil {
		t.Fatal("wrong-leg stop accepted")
	}
	if err := s.StopPlayback(context.Background(), "call", "customer", "a"); err != nil {
		t.Fatal(err)
	}
	if r.playbacks["b"] == nil || r.playbacks["a"] != nil {
		t.Fatal("stop affected another playback")
	}
}

func TestApplicationSessionLifecycleAndDirections(t *testing.T) {
	ctx := context.Background()
	r := &room{mixAudio: true, mixer: newScheduledRoomMixer(nil), peers: map[string]*peer{}, legRoles: map[string]dto.LegRole{}}
	s := &Service{rooms: map[string]*room{"call": r}}
	opts := ports.MediaStreamOptions{Direction: "sendonly", Input: ports.PCMFormat{SampleRate: 8000}, Output: ports.PCMFormat{SampleRate: 24000}}
	a, err := s.OpenApplicationStream(ctx, "call", "agent", opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.OpenApplicationStream(ctx, "call", "agent", opts); err == nil {
		t.Fatal("duplicate endpoint accepted")
	}
	if err = a.Write(1, []byte{1}); err == nil {
		t.Fatal("partial PCM sample accepted")
	}
	if err = a.Write(1, make([]byte, 48000+2)); err == nil {
		t.Fatal("oversized block accepted")
	}
	r.applicationFrames(map[string][]int16{}, time.Now())
	select {
	case <-a.Input():
		t.Fatal("send-only consumed customer audio")
	default:
	}
	if err = s.LeaveRoom(ctx, "call", "agent"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.Done():
	default:
		t.Fatal("removed leg leaked its media session")
	}
	opts.Direction = "duplex"
	a, err = s.OpenApplicationStream(ctx, "call", "agent", opts)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CloseRoom(ctx, "call"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.Done():
	default:
		t.Fatal("room close leaked its media session")
	}
}
