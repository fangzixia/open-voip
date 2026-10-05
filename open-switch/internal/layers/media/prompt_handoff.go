package media

import (
	"context"
	"time"

	"open-switch/internal/errs"
	"open-switch/internal/observability"
	"open-switch/internal/ports/dto"
)

const promptFrameDur = 20 * time.Millisecond

func durationToFadeFrames(d time.Duration) int {
	n := int(d / promptFrameDur)
	if n < 1 {
		n = 1
	}
	return n
}

func scaleMulawFrame(mulaw []byte, gain int32) []byte {
	if gain >= 32767 || len(mulaw) == 0 {
		return mulaw
	}
	if gain <= 0 {
		return make([]byte, len(mulaw))
	}
	pcm := pcmuPayloadToPCM(mulaw)
	for i := range pcm {
		pcm[i] = int16(int32(pcm[i]) * gain / 32767)
	}
	return pcmToG711(pcm, 0)
}

func (r *room) clearHandoffLocked() {
	r.connectGraceUntil = time.Time{}
	r.promptStopAt = time.Time{}
	r.promptFadeTotal = 0
	r.promptFadeLeft = 0
}

func (r *room) legRole(legID string) dto.LegRole {
	if p := r.peers[legID]; p != nil {
		return p.role
	}
	if r.legRoles != nil {
		if role, ok := r.legRoles[legID]; ok {
			return role
		}
	}
	return dto.LegRoleCustomer
}

func isCustomerRole(role dto.LegRole) bool {
	return role == dto.LegRoleCustomer || role == dto.LegRolePSTN || role == dto.LegRoleIVRBot
}

// mediaForwardAllowed 在直连桥接规则之上应用排队接听门禁（坐席→主叫）。
func (r *room) mediaForwardAllowed(from, to string) bool {
	if !r.canForward(from, to) {
		return false
	}
	r.mu.RLock()
	grace := r.connectGraceUntil
	r.mu.RUnlock()
	if grace.IsZero() || !time.Now().Before(grace) {
		return true
	}
	fromRole := r.legRole(from)
	toRole := r.legRole(to)
	return fromRole != dto.LegRoleAgent || !isCustomerRole(toRole)
}

// promptGainAndContinue 在 handoff/淡出阶段计算帧增益；cont=false 表示本帧为最后一帧。
func (r *room) promptGainAndContinue(seq uint64) (gain int32, cont bool) {
	if r.promptSeq.Load() != seq {
		return 0, false
	}
	r.mu.Lock()
	now := time.Now()
	if r.promptFadeLeft > 0 {
		left := r.promptFadeLeft
		r.promptFadeLeft--
		total := r.promptFadeTotal
		if total < 1 {
			total = 1
		}
		gain := int32(left) * 32767 / int32(total)
		last := left == 1
		if last {
			r.promptSeq.Add(1)
			r.clearHandoffLocked()
		}
		r.mu.Unlock()
		return gain, !last
	}
	if !r.promptStopAt.IsZero() && !now.Before(r.promptStopAt) && r.promptFadeTotal > 0 {
		r.promptFadeLeft = r.promptFadeTotal
	}
	r.mu.Unlock()
	return 32767, true
}

func (s *Service) beginPromptFade(callID string, r *room, stopAt time.Time) {
	frames := durationToFadeFrames(s.queueAnswerFade)
	r.mu.Lock()
	r.promptStopAt = stopAt
	r.promptFadeTotal = frames
	if r.promptFadeLeft == 0 {
		r.promptFadeLeft = frames
	}
	seq := r.promptSeq.Load()
	r.mu.Unlock()
	s.schedulePromptSeqBump(callID, seq, time.Duration(frames)*promptFrameDur+50*time.Millisecond)
}

func (s *Service) schedulePromptSeqBump(callID string, ifSeq uint64, wait time.Duration) {
	time.Sleep(wait)
	r := s.getRoom(callID)
	if r == nil {
		return
	}
	if r.promptSeq.Load() != ifSeq {
		return
	}
	r.promptSeq.Add(1)
	r.mu.Lock()
	r.clearHandoffLocked()
	r.mu.Unlock()
}

func (s *Service) handoffWatchdog(callID string, gen uint64, wait time.Duration, ifSeq uint64) {
	time.Sleep(wait)
	r := s.getRoom(callID)
	if r == nil {
		return
	}
	if r.handoffGen.Load() != gen {
		return
	}
	if r.promptSeq.Load() != ifSeq {
		return
	}
	r.promptSeq.Add(1)
	r.mu.Lock()
	r.clearHandoffLocked()
	r.mu.Unlock()
}

// BeginQueueAnswerHandoff 排队接听：保留等待音 grace 时长，再淡出停止注入，并短暂屏蔽坐席→主叫转发。
func (s *Service) BeginQueueAnswerHandoff(ctx context.Context, callID string, grace, fade time.Duration) error {
	if grace <= 0 {
		grace = s.queueAnswerGrace
	}
	if fade <= 0 {
		fade = s.queueAnswerFade
	}
	r := s.getRoom(callID)
	if r == nil {
		return errs.NotFound("媒体房间不存在")
	}
	fadeFrames := durationToFadeFrames(fade)
	now := time.Now()
	r.mu.Lock()
	if !r.connectGraceUntil.IsZero() && now.Before(r.connectGraceUntil) {
		r.mu.Unlock()
		return nil
	}
	startSeq := r.promptSeq.Load()
	r.connectGraceUntil = now.Add(grace)
	r.promptStopAt = now.Add(grace)
	r.promptFadeTotal = fadeFrames
	r.promptFadeLeft = 0
	gen := r.handoffGen.Add(1)
	r.mu.Unlock()
	go s.handoffWatchdog(callID, gen, grace+fade+50*time.Millisecond, startSeq)
	observability.Event(observability.WithFields(ctx, observability.Fields{CallID: callID}), "media", "prompt.handoff", "start", "ok", "", time.Time{},
		"grace_ms", grace.Milliseconds(), "fade_ms", fade.Milliseconds())
	return nil
}
