package media

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"net"
	"open-switch/internal/observability"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"

	"open-switch/internal/ports/dto"
)

type sipRTP struct {
	ua                                     *sipUA
	boundPort                              int
	callID                                 string
	legID                                  string
	held                                   bool
	muted                                  bool
	mu                                     sync.Mutex
	conn                                   *net.UDPConn
	remote                                 *net.UDPAddr
	remotePT                               uint8
	remoteCodec                            sipAudioCodec
	started                                time.Time
	rxPackets, rxBytes, txPackets, txBytes uint64
	sequenceGaps, outOfOrder               uint64
	expectedSeq                            uint16
	haveSeq                                bool
	summaryLogged                          bool
	inboundReady                           chan struct{}
	inboundOnce                            sync.Once
	txDroppedNoRemote                      uint64
}

func (u *sipUA) listenRTP() (*sipRTP, error) {
	minP := int(u.cfg.RTPPortMin)
	maxP := int(u.cfg.RTPPortMax)
	if minP <= 0 || maxP <= minP {
		minP, maxP = 20000, 20100
	}
	var last error
	for p := minP; p <= maxP; p++ {
		if !u.claimRTPPort(p) {
			continue
		}
		c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: p})
		if err != nil {
			u.releaseRTPPort(p)
			last = err
			continue
		}
		return &sipRTP{
			ua: u, boundPort: p, conn: c, remotePT: 0, started: time.Now().UTC(),
			inboundReady: make(chan struct{}),
		}, nil
	}
	return nil, last
}

func (r *sipRTP) close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	conn, ua, port := r.conn, r.ua, r.boundPort
	r.conn = nil
	already := r.summaryLogged
	r.summaryLogged = true
	callID, legID, started := r.callID, r.legID, r.started
	codec := r.codecLocked()
	rxPackets, rxBytes, txPackets, txBytes := r.rxPackets, r.rxBytes, r.txPackets, r.txBytes
	sequenceGaps, outOfOrder := r.sequenceGaps, r.outOfOrder
	r.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	if ua != nil {
		ua.releaseRTPPort(port)
	}
	if !already {
		ctx := observability.WithFields(context.Background(), observability.Fields{CallID: callID, LegID: legID})
		observability.Event(ctx, "sip_rtp", "rtp.summary", "hangup", "ok", "", started, "rx_packets", rxPackets, "rx_bytes", rxBytes, "tx_packets", txPackets, "tx_bytes", txBytes, "sequence_gaps", sequenceGaps, "out_of_order", outOfOrder, "codec_negotiated", codecName(codec), "sample_rate_hz", codec.sampleRate())
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

func (r *sipRTP) setRemoteCodec(codec sipAudioCodec, pt uint8) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.remoteCodec = codec
	r.remotePT = pt
	r.mu.Unlock()
}

func (r *sipRTP) currentCodec() sipAudioCodec {
	if r == nil {
		return sipCodecPCMU
	}
	r.mu.Lock()
	c := r.codecLocked()
	r.mu.Unlock()
	return c
}

func (r *sipRTP) codecLocked() sipAudioCodec {
	c := r.remoteCodec
	pt := r.remotePT
	if c == 0 {
		return codecFromPT(pt)
	}
	return c
}

func (r *sipRTP) write(b []byte) {
	if r == nil || len(b) == 0 {
		return
	}
	r.mu.Lock()
	conn, addr := r.conn, r.remote
	r.mu.Unlock()
	if conn == nil || addr == nil {
		atomic.AddUint64(&r.txDroppedNoRemote, 1)
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	n, err := conn.WriteToUDP(b, addr)
	if err == nil {
		r.mu.Lock()
		r.txPackets++
		r.txBytes += uint64(n)
		r.mu.Unlock()
	}
}

func digitToDTMFEvent(d string) (byte, bool) {
	if len(d) != 1 {
		return 0, false
	}
	switch d[0] {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return d[0] - '0', true
	case '*':
		return 10, true
	case '#':
		return 11, true
	default:
		return 0, false
	}
}

// sendRFC4733 向 SIP 对端发送 RFC 2833 电话按键（PT=101）。
func (r *sipRTP) sendRFC4733(digit string) {
	ev, ok := digitToDTMFEvent(digit)
	if !ok || r == nil {
		return
	}
	payload := []byte{ev, 0x8a, 0, 0}
	binary.BigEndian.PutUint16(payload[2:], 160*8)
	pkt := rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    101,
			SequenceNumber: uint16(rand.Uint32()),
			Timestamp:      rand.Uint32(),
			SSRC:           rand.Uint32(),
		},
		Payload: payload,
	}
	if raw, err := pkt.Marshal(); err == nil {
		r.write(raw)
	}
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

func applyRemoteSDP(rtpSess *sipRTP, media sdpMedia) error {
	if rtpSess == nil {
		return fmt.Errorf("SIP RTP session missing")
	}
	if !media.hasAudioCodec() {
		return fmt.Errorf("no common audio codec: PCMA/PCMU required")
	}
	if media.IP != "" && media.Port > 0 {
		rtpSess.latch(&net.UDPAddr{IP: net.ParseIP(media.IP), Port: media.Port})
	}
	audioProfile := ""
	if rtpSess.ua != nil && rtpSess.ua.media != nil && rtpSess.callID != "" {
		audioProfile = rtpSess.ua.media.roomAudioProfile(rtpSess.callID)
	}
	codec, pt := media.negotiateCodec()
	rtpSess.setRemoteCodec(codec, pt)
	if ua := rtpSess.ua; ua != nil && ua.media != nil {
		if room := ua.media.getRoom(rtpSess.callID); room != nil {
			room.mu.RLock()
			mixer := room.mixer
			room.mu.RUnlock()
			mixer.removeLeg(rtpSess.legID)
		}
	}
	logCodecNegotiation(rtpSess, media, codec, pt, audioProfile, "sdp")
	return nil
}

func codecName(c sipAudioCodec) string {
	switch c {
	case sipCodecPCMU:
		return "PCMU"
	case sipCodecPCMA:
		return "PCMA"
	default:
		return "UNKNOWN"
	}
}

func (s *Service) attachSIPRTP(callID string, rtpSess *sipRTP) {
	rtpSess.mu.Lock()
	rtpSess.callID = callID
	rtpSess.mu.Unlock()
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
	if r.legRoles == nil {
		r.legRoles = map[string]dto.LegRole{}
	}
	if rtpSess.legID != "" && r.legRoles[rtpSess.legID] == "" {
		r.legRoles[rtpSess.legID] = dto.LegRoleCustomer
	}
	r.mu.Unlock()
}

// sipReadLoop 校验 RTP 来源，解码电话按键，并在 SIP、WebRTC 与录音之间转发音频。
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
		rtpSess.observeInbound(pkt.SequenceNumber, n)
		// RFC 2833 结束包可能重复发送，同一时间戳只派发一次按键。
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
		if pkt.PayloadType != 101 {
			rtpSess.signalInbound()
		}
		if rtpSess.blocked() {
			continue
		}
		if pkt.PayloadType != rtpSess.currentPT() {
			continue
		}
		codec := rtpSess.currentCodec()
		if codec != sipCodecPCMU && codec != sipCodecPCMA {
			continue
		}
		if codec == sipCodecPCMA {
			pkt.PayloadType = 8
		} else {
			pkt.PayloadType = 0
		}
		r := s.getRoom(callID)
		if r == nil {
			continue
		}
		r.mu.RLock()
		mix := r.mixer
		r.mu.RUnlock()
		if mix != nil {
			mix.ingestPacket(rtpSess.legID, pkt)
		}
	}
}

// observeInbound 用 RTP 序号累计丢包间隔和乱序包，供媒体质量统计。
func (r *sipRTP) observeInbound(seq uint16, bytes int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rxPackets++
	r.rxBytes += uint64(bytes)
	if !r.haveSeq {
		r.haveSeq, r.expectedSeq = true, seq+1
		return
	}
	if seq == r.expectedSeq {
		r.expectedSeq++
		return
	}
	ahead := uint16(seq - r.expectedSeq)
	if ahead < 0x8000 {
		r.sequenceGaps += uint64(ahead)
		r.expectedSeq = seq + 1
	} else {
		r.outOfOrder++
	}
}

// dispatchDTMF 复制回调列表后异步调用，避免回调期间持有房间读锁或阻塞 RTP 读循环。
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
	if len(handlers) == 0 {
		return
	}
	r.enqueueDTMF(func() {
		for _, h := range handlers {
			h(context.Background(), dto.DTMFDigit(digit))
		}
	})
}

func (r *sipRTP) blocked() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.held || r.muted || r.conn == nil
}

// signalInbound 在收到对端首包媒体后关闭 inboundReady，供 IVR 在播首句前等待对称 RTP/终端开音。
func (r *sipRTP) signalInbound() {
	if r == nil {
		return
	}
	r.inboundOnce.Do(func() {
		if r.inboundReady != nil {
			close(r.inboundReady)
		}
	})
}

func (r *sipRTP) waitInbound(timeout time.Duration) bool {
	if r == nil {
		return false
	}
	ch := r.inboundReady
	if ch == nil {
		return true
	}
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (r *sipRTP) waitRemote(timeout time.Duration) bool {
	if r == nil {
		return false
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		ok := r.conn != nil && r.remote != nil
		r.mu.Unlock()
		if ok {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// acceptSource 对称 RTP：锁定首个有效源，同 IP 下允许端口随 NAT 变化。
func (r *sipRTP) acceptSource(addr *net.UDPAddr) bool {
	if addr == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn == nil {
		return false
	}
	if r.remote == nil {
		r.remote = &net.UDPAddr{IP: append(net.IP(nil), addr.IP...), Port: addr.Port}
		return true
	}
	if !addr.IP.Equal(r.remote.IP) {
		return false
	}
	if addr.Port != r.remote.Port {
		r.remote = &net.UDPAddr{IP: append(net.IP(nil), addr.IP...), Port: addr.Port}
	}
	return true
}
