package media

import (
	"sync"
	"time"

	"github.com/pion/rtp"

	"open-switch/internal/ports/dto"
)

// scheduledRoomMixer 用 RTP 时间轴缓冲各腿 PCM，由 20 ms 节拍统一混音下发。
type scheduledRoomMixer struct {
	mu sync.Mutex

	legs   map[string]*legPlayoutBuffer
	outSeq map[string]uint16
	outTS  map[string]uint32
	diag   *rtpPtimeDiag

	stopOnce sync.Once
	stopCh   chan struct{}
}

func newScheduledRoomMixer(diag *rtpPtimeDiag) *scheduledRoomMixer {
	return &scheduledRoomMixer{
		legs:   map[string]*legPlayoutBuffer{},
		outSeq: map[string]uint16{},
		outTS:  map[string]uint32{},
		diag:   diag,
		stopCh: make(chan struct{}),
	}
}

func (m *scheduledRoomMixer) stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() { close(m.stopCh) })
}

func (m *scheduledRoomMixer) done() <-chan struct{} {
	if m == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return m.stopCh
}

func (m *scheduledRoomMixer) ingest(legID string, seq uint16, ts, ssrc uint32, clockRate int, pcm []int16) {
	if m == nil || legID == "" || len(pcm) == 0 {
		return
	}
	pcm8 := pcmToMixInternal(pcm, clockRate)
	ts8 := tsToMixInternal(ts, clockRate)
	m.mu.Lock()
	leg := m.legs[legID]
	if leg == nil {
		leg = newLegPlayoutBuffer(defaultPlayoutCap)
		m.legs[legID] = leg
	}
	m.mu.Unlock()
	leg.ingest(seq, ssrc, ts8, pcm8)
	if m.diag != nil {
		m.diag.observe(legID, ts, len(pcm), clockRate)
	}
}

func (m *scheduledRoomMixer) advanceTick() map[string][]int16 {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string][]int16, len(m.legs))
	for id, leg := range m.legs {
		out[id] = leg.pullFrame()
	}
	return out
}

func (m *scheduledRoomMixer) nextRTP(legID string, pcmu []byte) *rtp.Packet {
	m.mu.Lock()
	defer m.mu.Unlock()
	seq := m.outSeq[legID]
	ts := m.outTS[legID]
	m.outSeq[legID] = seq + 1
	m.outTS[legID] = ts + uint32(len(pcmu))
	return &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0,
			SequenceNumber: seq,
			Timestamp:      ts,
			SSRC:           1,
		},
		Payload: pcmu,
	}
}

func (s *Service) runRoomMixLoop(callID string, r *room, mix *scheduledRoomMixer) {
	ticker := time.NewTicker(rtpFrameDur)
	defer ticker.Stop()
	for {
		select {
		case <-mix.done():
			return
		case <-ticker.C:
			s.dispatchMixTick(callID, r, mix)
		}
	}
}

func (s *Service) dispatchMixTick(callID string, r *room, mix *scheduledRoomMixer) {
	if mix == nil || r == nil {
		return
	}
	frames := mix.advanceTick()
	r.mu.Lock()
	if !r.mixAudio {
		targeted := r.applicationFrames(map[string][]int16{}, time.Now())
		r.dispatchDirectPlayback(mix, targeted)
		r.mu.Unlock()
		return
	}
	targeted := r.applicationFrames(frames, time.Now())
	rec := r.rec
	for id, p := range r.peers {
		if p.held {
			continue
		}
		mixed := addTargetAudio(mixPCMFramesLimited(frames, id), targeted[id])
		if rec != nil && rec.tapRecording() {
			role := p.role
			if role == "" {
				role = r.legRoles[id]
			}
			rec.TapMainMixed(role, mixed, mixInternalRate)
		}
		if p.audioOut == nil {
			continue
		}
		pcm8 := mixed
		if len(pcm8) > mixFrameSamples {
			pcm8 = pcm8[:mixFrameSamples]
		}
		pcmu := pcmToPCMU(pcm8)
		outPkt := mix.nextRTP(id, pcmu)
		if raw, err := outPkt.Marshal(); err == nil {
			_, _ = p.audioOut.Write(raw)
		}
	}
	for rt := range r.sipRTP {
		if rt.blocked() {
			continue
		}
		mixed := addTargetAudio(mixPCMFramesLimited(frames, rt.legID), targeted[rt.legID])
		if rec != nil && rec.tapRecording() {
			role := r.legRoles[rt.legID]
			if role == "" {
				role = dto.LegRoleCustomer
			}
			rec.TapMainMixed(role, mixed, mixInternalRate)
		}
		pcm8 := mixed
		if len(pcm8) > mixFrameSamples {
			pcm8 = pcm8[:mixFrameSamples]
		}
		pcmu := pcmToPCMU(pcm8)
		outPkt := mix.nextRTP(rt.legID, pcmu)
		if raw, err := outPkt.Marshal(); err == nil {
			rt.writePCMU(raw)
		}
	}
	r.mu.Unlock()
}
