package media

import (
	"context"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"open-switch/internal/ports/dto"
)

// forward 处理某一 leg 的入站 RTP。
//
// 若房间开启 mixAudio（典型：SIP 主叫 + WebRTC 坐席/AI）：
//   各 leg PCM 进入 RTP 时间轴缓冲，20 ms 调度混音后下发「除自己外的混音」，避免回声。
// 否则：原样转发 RTP 包。录音侧始终 writeRTP，由 recorder 解码为线性 PCM 混音。
func (s *Service) forward(callID, fromLeg string, remote *webrtc.TrackRemote) {
	buf := make([]byte, 1500)
	pkt := &rtp.Packet{}
	for {
		n, _, err := remote.Read(buf)
		if err != nil {
			return
		}
		r := s.getRoom(callID)
		if r == nil {
			return
		}
		r.mu.RLock()
		unmarshaled := pkt.Unmarshal(buf[:n]) == nil
		from := r.peers[fromLeg]
		muted := false
		held := false
		if from != nil {
			held = from.held
			if remote.Kind() == webrtc.RTPCodecTypeAudio {
				muted = from.audioMuted
			} else {
				muted = from.videoMuted
			}
			if from.role == dto.LegRoleSupervisor {
				muted = true
			}
		}
		if unmarshaled && !muted && !held && remote.Kind() == webrtc.RTPCodecTypeAudio && r.mixer != nil && r.mixAudio {
			if pkt.PayloadType != 101 { // 101=telephone-event，不参与语音混音
				pcmu := rtpPayloadToPCMUForLeg(fromLeg, pkt.PayloadType, append([]byte(nil), pkt.Payload...))
				if len(pcmu) == 0 {
					r.mu.RUnlock()
					continue
				}
				pcm := pcmuPayloadToPCM(pcmu)
				clock := 8000
				if len(pcm) > 0 {
					if r.rec != nil && r.rec.tapRecording() {
						role := dto.LegRole("")
						if from != nil {
							role = from.role
						}
						if role == "" {
							role = r.legRoles[fromLeg]
						}
						r.rec.TapUplink(fromLeg, role, pcm, clock)
					}
					r.mixer.ingest(fromLeg, pkt.SequenceNumber, pkt.Timestamp, pkt.SSRC, clock, pcm)
				}
			}
			r.mu.RUnlock()
			continue
		}
		var bridgePCM []int16
		bridgeTap := unmarshaled && !muted && !held && remote.Kind() == webrtc.RTPCodecTypeAudio &&
			!r.mixAudio && r.rec != nil && r.rec.tapRecording() && pkt.PayloadType != 101
		if bridgeTap {
			pcmu := rtpPayloadToPCMUForLeg(fromLeg, pkt.PayloadType, append([]byte(nil), pkt.Payload...))
			if len(pcmu) > 0 {
				bridgePCM = pcmuPayloadToPCM(pcmu)
				role := dto.LegRole("")
				if from != nil {
					role = from.role
				}
				if role == "" {
					role = r.legRoles[fromLeg]
				}
				r.rec.TapUplink(fromLeg, role, bridgePCM, 8000)
			} else {
				bridgeTap = false
			}
		}
		for id, p := range r.peers {
			if id == fromLeg || muted || held || p.held || !r.mediaForwardAllowed(fromLeg, id) {
				continue
			}
			var out *webrtc.TrackLocalStaticRTP
			if remote.Kind() == webrtc.RTPCodecTypeAudio {
				out = p.audioOut
			} else {
				out = p.videoOut
			}
			if out != nil {
				if bridgeTap && len(bridgePCM) > 0 {
					destRole := p.role
					if destRole == "" {
						destRole = r.legRoles[id]
					}
					r.rec.TapMainMixed(destRole, bridgePCM, 8000)
				}
				_, _ = out.Write(buf[:n])
			}
		}
		if unmarshaled && !muted && !held && remote.Kind() == webrtc.RTPCodecTypeAudio && r.sipRTP != nil {
			if pkt.PayloadType == 101 {
				r.mu.RUnlock()
				continue
			}
			pcmu := rtpPayloadToPCMU(pkt.PayloadType, append([]byte(nil), pkt.Payload...))
			if len(pcmu) == 0 {
				r.mu.RUnlock()
				continue
			}
			outPkt := rtp.Packet{
				Header: rtp.Header{Version: 2, PayloadType: 0, SequenceNumber: pkt.SequenceNumber, Timestamp: pkt.Timestamp, SSRC: pkt.SSRC},
				Payload: pcmu,
			}
			if raw, err := outPkt.Marshal(); err == nil {
				for rt := range r.sipRTP {
					if !rt.blocked() && r.canForward(fromLeg, rt.legID) {
						rt.writePCMU(raw)
					}
				}
			}
		}
		r.mu.RUnlock()
	}
}

// readDTMF 从 WebRTC RTP 轨道提取电话按键事件并通知订阅者。
func (s *Service) readDTMF(callID, legID string, remote *webrtc.TrackRemote) {
	buf := make([]byte, 1500)
	last := byte(255)
	for {
		n, _, err := remote.Read(buf)
		if err != nil {
			return
		}
		if n < 1 {
			continue
		}
		pkt := &rtp.Packet{}
		if err := pkt.Unmarshal(buf[:n]); err != nil || len(pkt.Payload) < 1 {
			continue
		}
		event := pkt.Payload[0] & 0x7f
		end := len(pkt.Payload) > 1 && pkt.Payload[1]&0x80 != 0
		if !end {
			continue
		}
		if event == last {
			continue
		}
		last = event
		digit := dtmfEvent(event)
		r := s.getRoom(callID)
		if r == nil {
			return
		}
		r.mu.RLock()
		h := r.dtmf[legID]
		r.mu.RUnlock()
		if h != nil && digit != "" {
			r.enqueueDTMF(func() { h(context.Background(), dto.DTMFDigit(digit)) })
			last = 255
		}
	}
}

func dtmfEvent(ev byte) string {
	if ev <= 9 {
		return string('0' + ev)
	}
	switch ev {
	case 10:
		return "*"
	case 11:
		return "#"
	default:
		return ""
	}
}
