package media

import (
	"context"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"open-switch/internal/ports/dto"
)

// forward 将输入 RTP 轨道转发给房间内其他通话腿，并写入录音。
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
		if unmarshaled && r.rec != nil {
			cp := *pkt
			cp.Payload = append([]byte{}, pkt.Payload...)
			r.rec.writeRTP(fromLeg, remote.Kind(), remote.Codec().MimeType, &cp)
		}
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
			if pkt.PayloadType != 101 {
				pcmu := rtpPayloadToPCMU(pkt.PayloadType, append([]byte(nil), pkt.Payload...))
				if len(pcmu) > 0 {
					r.mixer.ingest(fromLeg, pcmu)
					for id, p := range r.peers {
						if id == fromLeg || p.held || !r.canForward(fromLeg, id) {
							continue
						}
						mixed := pcmToPCMU(r.mixer.mixExcept(id))
						if p.audioOut != nil {
							outPkt := r.mixer.nextRTP(id, mixed)
							if raw, err := outPkt.Marshal(); err == nil {
								_, _ = p.audioOut.Write(raw)
							}
						}
					}
					for rt := range r.sipRTP {
						if rt.blocked() || !r.canForward(fromLeg, rt.legID) {
							continue
						}
						mixed := pcmToPCMU(r.mixer.mixExcept(rt.legID))
						outPkt := r.mixer.nextRTP(rt.legID, mixed)
						if raw, err := outPkt.Marshal(); err == nil {
							rt.writePCMU(raw)
						}
					}
				}
			}
			r.mu.RUnlock()
			continue
		}
		for id, p := range r.peers {
			if id == fromLeg || muted || held || p.held || !r.canForward(fromLeg, id) {
				continue
			}
			var out *webrtc.TrackLocalStaticRTP
			if remote.Kind() == webrtc.RTPCodecTypeAudio {
				out = p.audioOut
			} else {
				out = p.videoOut
			}
			if out != nil {
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
