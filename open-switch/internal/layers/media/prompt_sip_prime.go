package media

import (
	"context"
	"time"

	"open-switch/internal/observability"
)

const (
	// 呼入 SIP IVR：等待对端首包 RTP，给终端/对称 RTP 留出建立时间。
	promptSIPInboundWait = 1500 * time.Millisecond
	// 正式播报前发送的 G.711 静音帧数（20ms/帧），补偿摘机后解码器未就绪导致的丢首字。
	promptSIPLeadInFrames = 10
)

func mulawSilenceFrame() []byte {
	b := make([]byte, 160)
	for i := range b {
		b[i] = 0xff
	}
	return b
}

func (s *Service) sipRTPsForTarget(callID, targetLegID string) []*sipRTP {
	r := s.getRoom(callID)
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*sipRTP, 0, len(r.sipRTP))
	for rt := range r.sipRTP {
		if targetLegID != "" && rt.legID != targetLegID {
			continue
		}
		out = append(out, rt)
	}
	return out
}

// shouldPrimeSIPPrompt 仅对主叫/PSTN 的 SIP 腿做开音前导，避免保持音、坐席腿等多等 1.5s。
func (s *Service) shouldPrimeSIPPrompt(callID, targetLegID string) bool {
	r := s.getRoom(callID)
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.sipRTP) == 0 {
		return false
	}
	if targetLegID != "" {
		return isCustomerRole(r.legRole(targetLegID))
	}
	for rt := range r.sipRTP {
		if isCustomerRole(r.legRole(rt.legID)) {
			return true
		}
	}
	return false
}

// primeSIPPrompt 在 WAV/编码处理前向电话侧送静音前导，并尽量等待对端首包入向 RTP。
func (s *Service) primeSIPPrompt(ctx context.Context, callID, targetLegID string, seq uint64, rtpSeq *uint16, rtpTS *uint32, ssrc uint32, nextSend *time.Time, lateTotal *int) {
	if !s.shouldPrimeSIPPrompt(callID, targetLegID) {
		return
	}
	rts := s.sipRTPsForTarget(callID, targetLegID)
	if len(rts) == 0 {
		return
	}
	for _, rt := range rts {
		rt.waitRemote(500 * time.Millisecond)
	}
	inboundOK := false
	for _, rt := range rts {
		if rt.waitInbound(promptSIPInboundWait) {
			inboundOK = true
			break
		}
	}
	silence := mulawSilenceFrame()
	sent := 0
	for i := 0; i < promptSIPLeadInFrames; i++ {
		if !s.playOnePromptFrame(callID, targetLegID, seq, rtpSeq, rtpTS, ssrc, silence, nil, nextSend, lateTotal) {
			break
		}
		sent++
	}
	observability.Event(observability.WithFields(ctx, observability.Fields{CallID: callID}),
		"media", "prompt.prime", "sip", "ok", "", time.Now(),
		"lead_in_frames", sent, "inbound_seen", inboundOK, "target_leg_id", targetLegID)
}
