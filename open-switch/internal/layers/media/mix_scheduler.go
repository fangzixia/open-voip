package media

import (
	msdk "github.com/livekit/media-sdk"
	lkmixer "github.com/livekit/media-sdk/mixer"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media"

	"open-switch/internal/ports/dto"
)

// scheduledRoomMixer 用 RTP 时间轴缓冲各腿 PCM，由 20 ms 节拍统一混音下发。
type scheduledRoomMixer struct {
	scheduling durationHistogram
	mu         sync.Mutex

	legs    map[string]*legPlayoutBuffer
	outSeq  map[string]uint16
	outTS   map[string]uint32
	outSSRC map[string]uint32

	stopOnce sync.Once
	stopCh   chan struct{}
	clock    *lkmixer.Mixer
	inputs   map[string]*lkmixer.Input
	outputs  map[string]*mediaOutput
	epochs   map[string]uint64
	gains    map[string]*routeGain
}

func newScheduledRoomMixer() *scheduledRoomMixer {
	return &scheduledRoomMixer{
		legs:    map[string]*legPlayoutBuffer{},
		outSeq:  map[string]uint16{},
		outTS:   map[string]uint32{},
		outSSRC: map[string]uint32{},
		stopCh:  make(chan struct{}),
		inputs:  map[string]*lkmixer.Input{},
		outputs: map[string]*mediaOutput{},
		epochs:  map[string]uint64{},
		gains:   map[string]*routeGain{},
	}
}

func (m *scheduledRoomMixer) stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		close(m.stopCh)
		m.mu.Lock()
		clock := m.clock
		legs := make([]*legPlayoutBuffer, 0, len(m.legs))
		for _, leg := range m.legs {
			legs = append(legs, leg)
		}
		outputs := make([]*mediaOutput, 0, len(m.outputs))
		for _, out := range m.outputs {
			outputs = append(outputs, out)
		}
		m.mu.Unlock()
		if clock != nil {
			clock.StopAndWait()
		}
		for _, leg := range legs {
			leg.close()
		}
		for _, out := range outputs {
			out.stop()
		}
	})
}

func (m *scheduledRoomMixer) done() <-chan struct{} {
	if m == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return m.stopCh
}

func (m *scheduledRoomMixer) ingestPacket(id string, pkt *rtp.Packet) {
	if id == "" || (pkt.PayloadType != 0 && pkt.PayloadType != 8) || len(pkt.Payload) == 0 || len(pkt.Payload) > 320 {
		return
	}
	m.mu.Lock()
	select {
	case <-m.stopCh:
		m.mu.Unlock()
		return
	default:
	}
	leg := m.legs[id]
	if leg == nil {
		leg = newLegPlayoutBuffer(defaultPlayoutCap)
		m.legs[id] = leg
	}
	m.mu.Unlock()
	leg.push(pkt)
}

func (m *scheduledRoomMixer) advanceTick() map[string][]int16 {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string][]int16, len(m.legs))
	for id, leg := range m.legs {
		out[id] = leg.pullAdaptiveFrame()
	}
	return out
}

func (m *scheduledRoomMixer) nextRTP(legID string, pcmu []byte) *rtp.Packet {
	m.mu.Lock()
	defer m.mu.Unlock()
	seq := m.outSeq[legID]
	ts := m.outTS[legID]
	ssrc := m.outSSRC[legID]
	if ssrc == 0 {
		ssrc = rand.Uint32() | 1
		seq = uint16(rand.Uint32())
		ts = rand.Uint32()
		m.outSSRC[legID] = ssrc
	}
	m.outSeq[legID] = seq + 1
	m.outTS[legID] = ts + uint32(len(pcmu))
	return &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0,
			SequenceNumber: seq,
			Timestamp:      ts,
			SSRC:           ssrc,
		},
		Payload: pcmu,
	}
}

func (s *Service) runRoomMixLoop(callID string, r *room, mix *scheduledRoomMixer) {
	go s.sampleRoomQuality(callID, r, mix)
	clock, err := lkmixer.NewMixer(roomClockWriter{}, rtpFrameDur, 1,
		lkmixer.WithTimingHandler(mix.scheduling.observe),
		lkmixer.WithInputBufferFrames(10), lkmixer.WithInputBufferMin(0),
		lkmixer.WithFrameHandler(func() {
			frames := mix.advanceTick()
			mix.mu.Lock()
			if mix.clock == nil {
				mix.mu.Unlock()
				return
			}
			for id, frame := range frames {
				inp := mix.inputs[id]
				if inp == nil {
					inp = mix.clock.NewInput()
					mix.inputs[id] = inp
				}
				if inp == nil {
					continue
				}
				leg := mix.legs[id]
				if leg == nil {
					continue
				}
				leg.mu.Lock()
				epoch := leg.generation
				leg.mu.Unlock()
				if mix.epochs[id] != epoch {
					inp.Reset()
					mix.epochs[id] = epoch
				}
				_ = inp.WriteSample(msdk.PCM16Sample(frame))
			}
			mix.mu.Unlock()
		}, func(stems map[*lkmixer.Input]msdk.PCM16Sample) {
			frames := map[string][]int16{}
			mix.mu.Lock()
			for id, inp := range mix.inputs {
				frames[id] = stems[inp]
			}
			mix.mu.Unlock()
			s.dispatchMixFrames(callID, r, mix, frames)
		}))
	if err != nil {
		panic(err)
	} // mono 8 kHz is a static construction invariant.
	mix.mu.Lock()
	mix.clock = clock
	select {
	case <-mix.stopCh:
		clock.Stop()
	default:
	}
	mix.mu.Unlock()
	<-mix.done()
}

type roomClockWriter struct{}

func (roomClockWriter) SampleRate() int                    { return mixInternalRate }
func (roomClockWriter) String() string                     { return "room-clock" }
func (roomClockWriter) WriteSample(msdk.PCM16Sample) error { return nil }

func (s *Service) dispatchMixTick(callID string, r *room, mix *scheduledRoomMixer) {
	if mix == nil || r == nil {
		return
	}
	frames := mix.advanceTick()
	s.dispatchMixFrames(callID, r, mix, frames)
}

func (s *Service) dispatchMixFrames(callID string, r *room, mix *scheduledRoomMixer, frames map[string][]int16) {
	type destination struct {
		id    string
		frame []int16
		write func([]byte)
		pt    uint8
	}
	var outputs []destination
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	for id, p := range r.peers {
		if p.held || p.audioMuted || p.role == dto.LegRoleSupervisor {
			delete(frames, id)
		}
	}
	for rt := range r.sipRTP {
		if rt.blocked() {
			delete(frames, rt.legID)
		}
	}
	targeted := r.applicationFrames(frames, time.Now())
	prompts := map[string][]int16{}
	for _, pcm := range targeted {
		for source, frame := range pcm {
			prompts[source] = frame
		}
	}
	if pcm, target := s.promptFrameLocked(callID, r, time.Now()); len(pcm) > 0 {
		prompts["injected"] = pcm
		add := func(id string) {
			if targeted[id] == nil {
				targeted[id] = map[string][]int16{}
			}
			targeted[id]["injected"] = pcm
		}
		if target != "" {
			add(target)
		} else {
			for id := range r.peers {
				add(id)
			}
			for rt := range r.sipRTP {
				add(rt.legID)
			}
		}
	}
	rec := r.rec
	legs := make([]string, 0, len(r.peers)+len(r.sipRTP)+len(r.streams))
	for id := range r.peers {
		legs = append(legs, id)
	}
	for rt := range r.sipRTP {
		legs = append(legs, rt.legID)
	}
	for id := range r.streams {
		legs = append(legs, id)
	}
	routed := func(id string) []int16 {
		sources := map[string][]int16{}
		held := r.playbackTargetHeld(id)
		for from, pcm := range frames {
			if !held && from != id && r.mediaForwardAllowed(from, id) {
				sources[from] = pcm
			}
		}
		for source, pcm := range targeted[id] {
			sources["prompt:"+source] = pcm
		}
		mix.mu.Lock()
		g := mix.gains[id]
		if g == nil {
			g = &routeGain{}
			mix.gains[id] = g
		}
		out := g.mix(sources)
		mix.mu.Unlock()
		return out
	}
	for id, p := range r.peers {
		if !r.mixAudio {
			// Independent video/WebRTC retains its negotiated voice track.
			// Prompt samples use its existing separate G.711 track.
			if track := p.audioSamp; track != nil && !p.held && len(targeted[id]) > 0 {
				outputs = append(outputs, destination{id: "prompt:" + id, frame: routed(id), write: func(raw []byte) { _ = track.WriteSample(media.Sample{Data: raw, Duration: rtpFrameDur}) }, pt: 127})
			}
			continue
		}
		if p.audioOut == nil || (p.held && len(targeted[id]) == 0) {
			continue
		}
		track := p.audioOut
		outputs = append(outputs, destination{id: id, frame: routed(id), write: func(raw []byte) { _, _ = track.Write(raw) }})
	}
	for rt := range r.sipRTP {
		rt.mu.Lock()
		held, ready := rt.held, rt.conn != nil
		rt.mu.Unlock()
		if !ready || (held && len(targeted[rt.legID]) == 0) {
			continue
		}
		pt := rt.currentPT()
		if pt != 0 && pt != 8 {
			continue
		}
		outputs = append(outputs, destination{id: rt.legID, frame: routed(rt.legID), write: rt.write, pt: pt})
	}
	r.mu.Unlock()
	if rec != nil {
		rec.recordFrameGroup(frames, prompts, legs)
	}
	for _, out := range outputs {
		if out.pt == 127 {
			mix.send(out.id, pcmToPCMU(out.frame), out.write)
			continue
		}
		pkt := mix.nextRTP(out.id, pcmToG711(out.frame, out.pt))
		pkt.PayloadType = out.pt
		if raw, e := pkt.Marshal(); e == nil {
			mix.send(out.id, raw, out.write)
		}
	}
}
