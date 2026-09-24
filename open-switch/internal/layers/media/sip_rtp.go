package media

import (
	"context"
	"net"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"open-switch/internal/ports/dto"
)

type sipRTP struct {
	legID    string
	held     bool
	muted    bool
	mu       sync.Mutex
	conn     *net.UDPConn
	remote   *net.UDPAddr
	remotePT uint8
}

func (u *sipUA) listenRTP() (*sipRTP, error) {
	minP := int(u.cfg.RTPPortMin)
	maxP := int(u.cfg.RTPPortMax)
	if minP <= 0 || maxP <= minP {
		minP, maxP = 20000, 20100
	}
	var last error
	for p := minP; p <= maxP; p++ {
		c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: p})
		if err != nil {
			last = err
			continue
		}
		return &sipRTP{conn: c, remotePT: 0}, nil
	}
	return nil, last
}

func (r *sipRTP) close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil {
		_ = r.conn.Close()
		r.conn = nil
	}
}

func (r *sipRTP) latch(addr *net.UDPAddr) {
	if r == nil || addr == nil || addr.IP == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.remote = new(*addr)
}

func (r *sipRTP) currentPT() uint8 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.remotePT
}

func (r *sipRTP) setRemotePT(pt uint8) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if pt == 8 {
		r.remotePT = 8
	} else {
		r.remotePT = 0
	}
}

func (r *sipRTP) write(b []byte) {
	if r == nil || len(b) == 0 {
		return
	}
	r.mu.Lock()
	conn, addr := r.conn, r.remote
	r.mu.Unlock()
	if conn == nil || addr == nil {
		return
	}
	_, _ = conn.WriteToUDP(b, addr)
}

func (r *sipRTP) writePCMU(b []byte) {
	if r == nil || len(b) == 0 {
		return
	}
	pkt := &rtp.Packet{}
	if err := pkt.Unmarshal(b); err != nil {
		r.write(b)
		return
	}
	if pkt.PayloadType == 101 {
		r.write(b)
		return
	}
	r.mu.Lock()
	toPT := r.remotePT
	r.mu.Unlock()
	fromPT := pkt.PayloadType
	if fromPT != 0 && fromPT != 8 {
		fromPT = 0
	}
	pkt.Payload = transcodeG711(fromPT, toPT, pkt.Payload)
	pkt.PayloadType = toPT
	pkt.Extension = false
	pkt.Extensions = nil
	pkt.Padding = false
	raw, err := pkt.Marshal()
	if err != nil {
		return
	}
	r.write(raw)
}

func (r *sipRTP) localPort() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn == nil {
		return 0
	}
	a, ok := r.conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return 0
	}
	return a.Port
}

func applyRemoteSDP(rtpSess *sipRTP, media sdpMedia) {
	if rtpSess == nil {
		return
	}
	if media.IP != "" && media.Port > 0 {
		rtpSess.latch(&net.UDPAddr{IP: net.ParseIP(media.IP), Port: media.Port})
	}
	rtpSess.setRemotePT(media.preferG711())
}

func (s *Service) attachSIPRTP(callID string, rtpSess *sipRTP) {
	s.mu.Lock()
	r := s.rooms[callID]
	if r == nil {
		if s.sipRTPPending[callID] == nil {
			s.sipRTPPending[callID] = map[*sipRTP]struct{}{}
		}
		s.sipRTPPending[callID][rtpSess] = struct{}{}
		s.sipPending[callID] = true
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	r.mu.Lock()
	if r.sipRTP == nil {
		r.sipRTP = map[*sipRTP]struct{}{}
	}
	r.sipRTP[rtpSess] = struct{}{}
	r.sipAudio = true
	r.mu.Unlock()
}

func (s *Service) hasWebRTCPeer(callID string) bool {
	r := s.getRoom(callID)
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.peers) > 0
}

func (s *Service) sipReadLoop(callID string, rtpSess *sipRTP) {
	if rtpSess == nil {
		return
	}
	rtpSess.mu.Lock()
	conn := rtpSess.conn
	rtpSess.mu.Unlock()
	if conn == nil {
		return
	}
	buf := make([]byte, 1500)
	pkt := &rtp.Packet{}
	lastDTMF := uint32(0)
	haveDTMF := false
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if err := pkt.Unmarshal(buf[:n]); err != nil {
			continue
		}
		if !rtpSess.acceptSource(addr) {
			continue
		}
		if pkt.PayloadType == 101 && len(pkt.Payload) > 0 {
			event := pkt.Payload[0] & 0x7f
			end := len(pkt.Payload) > 1 && pkt.Payload[1]&0x80 != 0
			if end && (!haveDTMF || pkt.Timestamp != lastDTMF) {
				lastDTMF = pkt.Timestamp
				haveDTMF = true
				if d := dtmfEvent(event); d != "" {
					s.dispatchDTMF(callID, d)
				}
			}
			continue
		}
		if pkt.PayloadType != 0 && pkt.PayloadType != 8 {
			continue
		}
		if rtpSess.blocked() {
			continue
		}
		if pkt.PayloadType == 8 {
			pkt.Payload = transcodeG711(8, 0, pkt.Payload)
			pkt.PayloadType = 0
			pkt.Extension = false
			pkt.Extensions = nil
			pkt.Padding = false
		}
		raw, err := pkt.Marshal()
		if err != nil {
			raw = buf[:n]
		}
		r := s.getRoom(callID)
		if r == nil {
			continue
		}
		r.mu.RLock()
		for _, p := range r.peers {
			if p.audioOut != nil && !p.held {
				_, _ = p.audioOut.Write(raw)
			}
		}
		for dst := range r.sipRTP {
			if dst != rtpSess && !dst.blocked() {
				dst.writePCMU(raw)
			}
		}
		rec := r.rec
		r.mu.RUnlock()
		if rec != nil {
			cp := *pkt
			cp.Payload = append([]byte{}, pkt.Payload...)
			rec.writeRTP(webrtc.RTPCodecTypeAudio, &cp)
		}
	}
}

func (s *Service) dispatchDTMF(callID, digit string) {
	if digit == "" {
		return
	}
	r := s.getRoom(callID)
	if r == nil {
		return
	}
	r.mu.RLock()
	handlers := make([]func(context.Context, dto.DTMFDigit), 0, len(r.dtmf))
	for _, h := range r.dtmf {
		if h != nil {
			handlers = append(handlers, h)
		}
	}
	r.mu.RUnlock()
	for _, h := range handlers {
		h(context.Background(), dto.DTMFDigit(digit))
	}
}

func (r *sipRTP) blocked() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.held || r.muted || r.conn == nil
}

// 对称 RTP 可能改变协商端口，但绝不信任来源 IP 与远端不一致的数据包。
func (r *sipRTP) acceptSource(addr *net.UDPAddr) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if addr == nil || r.remote == nil || !addr.IP.Equal(r.remote.IP) {
		return false
	}
	r.remote = &net.UDPAddr{IP: append(net.IP(nil), addr.IP...), Port: addr.Port}
	return true
}
