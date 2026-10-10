package media

import (
	"context"
	"time"
)

type roomPrompt struct {
	target                      string
	pcm                         []int16
	offset, gapSamples, gapLeft int
	loop                        bool
	generation                  uint64
	readySince, startedAt       time.Time
}

func (s *Service) playPCMToRoom(callID, target string, pcm []int16, rate int, loop bool, seq uint64) {
	r := s.getRoom(callID)
	if r == nil {
		return
	}
	var e error
	pcm, e = resamplePCM(pcm, rate, 8000)
	if e != nil {
		if r.promptSeq.Load() == seq && s.onPromptFinished != nil {
			go s.onPromptFinished(context.Background(), callID, true)
		}
		return
	}
	pcm = preparePromptPCM(pcm)
	r.mu.Lock()
	if !r.closed && r.promptSeq.Load() == seq {
		r.prompt = &roomPrompt{target: target, pcm: pcm, loop: loop, generation: seq}
	}
	r.mu.Unlock()
}
func (s *Service) playWaitingTone(callID, target string, loop bool, seq uint64) {
	r := s.getRoom(callID)
	if r == nil {
		return
	}
	pcm := make([]int16, 8000)
	frame := pcmuPayloadToPCM(mulawToneFrame())
	for i := range pcm {
		pcm[i] = frame[i%len(frame)]
	}
	r.mu.Lock()
	if !r.closed && r.promptSeq.Load() == seq {
		r.prompt = &roomPrompt{target: target, pcm: pcm, loop: loop, generation: seq, gapSamples: 24000}
	}
	r.mu.Unlock()
}
func (s *Service) playToneToRoom(callID, target string, duration time.Duration, seq uint64, _ bool) {
	pcm := make([]int16, int(duration.Seconds()*8000))
	frame := pcmuPayloadToPCM(mulawToneFrame())
	for i := range pcm {
		pcm[i] = frame[i%len(frame)]
	}
	s.playPCMToRoom(callID, target, pcm, 8000, false, seq)
}

// Caller holds room.mu; frames go through the same output and recording tick.
func (s *Service) promptFrameLocked(callID string, r *room, now time.Time) ([]int16, string) {
	p := r.prompt
	if p == nil {
		return nil, ""
	}
	if p.generation != r.promptSeq.Load() {
		r.prompt = nil
		return nil, ""
	}
	// Silence continues on the room clock while endpoints establish media;
	// do not consume the first syllable before the recipient can receive it.
	ready := false
	if p.target != "" {
		ready = r.hasPlaybackTarget(p.target)
	} else {
		for id := range r.peers {
			ready = ready || r.hasPlaybackTarget(id)
		}
		for rt := range r.sipRTP {
			ready = ready || r.hasPlaybackTarget(rt.legID)
		}
	}
	if !ready {
		p.readySince = time.Time{}
		return nil, p.target
	}
	if p.readySince.IsZero() {
		p.readySince = now
	}
	if len(r.sipRTP) > 0 && now.Sub(p.readySince) < 200*time.Millisecond {
		return nil, p.target
	}
	if p.gapLeft > 0 {
		p.gapLeft -= mixFrameSamples
		return nil, p.target
	}
	if p.offset >= len(p.pcm) {
		if !p.loop {
			r.prompt = nil
			if s.onPromptFinished != nil {
				go s.onPromptFinished(context.Background(), callID, false)
			}
			return nil, p.target
		}
		p.offset = 0
		p.gapLeft = p.gapSamples
		if p.gapLeft > 0 {
			return nil, p.target
		}
	}
	gain := 32767
	if !r.promptStopAt.IsZero() && !now.Before(r.promptStopAt) && r.promptFadeLeft == 0 {
		r.promptFadeLeft = r.promptFadeTotal
	}
	if r.promptFadeLeft > 0 {
		gain = 32767 * r.promptFadeLeft / max(1, r.promptFadeTotal)
		r.promptFadeLeft--
		if r.promptFadeLeft == 0 {
			r.promptSeq.Add(1)
			r.clearHandoffLocked()
		}
	}
	pcm := make([]int16, mixFrameSamples)
	n := min(len(pcm), len(p.pcm)-p.offset)
	for i := 0; i < n; i++ {
		pcm[i] = int16(int32(p.pcm[p.offset+i]) * int32(gain) / 32767)
	}
	p.offset += n
	return pcm, p.target
}
