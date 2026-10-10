package media

import (
	"context"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"open-switch/internal/ports/dto"
)

// forward validates the negotiated codec before handing packets to the common
// pipeline. Independent video rooms retain their packet forwarding contract.
func (s *Service) forward(callID, fromLeg string, remote *webrtc.TrackRemote) {
	buf := make([]byte, 65535)
	for {
		n, _, e := remote.Read(buf)
		if e != nil {
			return
		}
		pkt := &rtp.Packet{}
		if pkt.Unmarshal(buf[:n]) != nil {
			continue
		}
		r := s.getRoom(callID)
		if r == nil {
			return
		}
		r.mu.RLock()
		from := r.peers[fromLeg]
		muted, held := false, false
		if from != nil {
			held = from.held
			if remote.Kind() == webrtc.RTPCodecTypeAudio {
				muted = from.audioMuted || from.role == dto.LegRoleSupervisor
			} else {
				muted = from.videoMuted
			}
		}
		mix, mixAudio, rec := r.mixer, r.mixAudio, r.rec
		var tracks []*webrtc.TrackLocalStaticRTP
		if !mixAudio || remote.Kind() != webrtc.RTPCodecTypeAudio {
			for id, p := range r.peers {
				if id == fromLeg || muted || held || p.held || !r.mediaForwardAllowed(fromLeg, id) {
					continue
				}
				out := p.videoOut
				if remote.Kind() == webrtc.RTPCodecTypeAudio {
					out = p.audioOut
				}
				if out != nil {
					tracks = append(tracks, out)
				}
			}
		}
		r.mu.RUnlock()
		if muted || held {
			continue
		}
		if remote.Kind() == webrtc.RTPCodecTypeAudio && mixAudio && mix != nil {
			codec := remote.Codec()
			if codec.ClockRate != 8000 || pkt.PayloadType != uint8(codec.PayloadType) {
				continue
			}
			switch codec.MimeType {
			case webrtc.MimeTypePCMU:
				pkt.PayloadType = 0
			case webrtc.MimeTypePCMA:
				pkt.PayloadType = 8
			default:
				continue
			}
			mix.ingestPacket(fromLeg, pkt)
			continue
		}
		if rec != nil {
			rec.writeRTP(fromLeg, remote.Kind(), remote.Codec().MimeType, pkt)
		}
		for _, out := range tracks {
			_, _ = out.Write(buf[:n])
		}
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
