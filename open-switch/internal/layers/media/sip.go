package media

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/google/uuid"

	"open-switch/internal/config"
	"open-switch/internal/observability"
	"open-switch/internal/ports"
)

const (
	sipSessionMinSE       = 90
	sipMaxRedirectHops    = 2
	sipPRACKWait          = 32 * time.Second
	sipRegisterMinExpires = 60
)

// InboundSIPHandler 由组合根注入：DID 呼入时创建 L3 通话。sourceID
// 是可信中继 ID；设备呼叫时为空。callID 为预分配 ID。
type InboundSIPHandler func(ctx context.Context, sourceID, destination, from, callID string) (gotCallID, legID string, err error)

// HangupSIPHandler 对端 BYE/超时取消时通知 L3。
type HangupSIPHandler func(ctx context.Context, callID string)

type sipBinding struct {
	AOR       string
	Contact   sip.Uri
	ExpiresAt time.Time
	CallID    string
	CSeq      uint32
	Addr      *net.UDPAddr
}

type sipUA struct {
	cfg      config.SIPConfig
	enabled  bool
	media    *Service
	mu       sync.Mutex
	onIn     InboundSIPHandler
	onDevice InboundSIPHandler
	onBye    HangupSIPHandler
	byCall   map[string]map[*sipSession]struct{}
	bySIP    map[string]*sipSession
	byDlg    map[string]*sipSession
	binds    map[string][]sipBinding
	bindings ports.SIPBindingStore
	nonces   map[string]time.Time
	allowed  []*net.IPNet
	dnsCache map[string]dnsCacheEntry
	rtpUsed  map[int]struct{}
	cli      *sipgo.Client
	dlgCli   *sipgo.DialogClientCache
	dlgSrv   *sipgo.DialogServerCache
}

type sipSession struct {
	mu          sync.Mutex
	callID      string
	sipCallID   string
	dlgID       string
	peerIP      net.IP
	rtp         *sipRTP
	client      *sipgo.DialogClientSession
	server      *sipgo.DialogServerSession
	ended       bool
	confirmed   bool
	cancel      context.CancelFunc
	inviteCSeq  uint32
	rseq        uint32
	prackCh     chan struct{}
	sessionExp  time.Duration
	refreshStop chan struct{}
	refreshOnce sync.Once
}

func newSIPUA(cfg config.SIPConfig, media *Service) *sipUA {
	return &sipUA{
		cfg:     cfg,
		enabled: cfg.Enabled,
		media:   media,
		byCall:  map[string]map[*sipSession]struct{}{},
		bySIP:   map[string]*sipSession{},
		byDlg:   map[string]*sipSession{},
		binds:   map[string][]sipBinding{},
		nonces:  map[string]time.Time{},
	}
}

// SetInboundHandler 注册 DID 呼入回调（仅组合根调用）。
func (s *Service) SetInboundHandler(h InboundSIPHandler) {
	if s.sip != nil {
		s.sip.onIn = h
	}
}

// SetSIPHangupHandler 注册 SIP 侧挂断回调（仅组合根调用）。
func (s *Service) SetSIPHangupHandler(h HangupSIPHandler) {
	if s.sip != nil {
		s.sip.onBye = h
	}
}

func (s *Service) enableSIPAudio(callID string) {
	if callID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.rooms[callID]; r != nil {
		r.mu.Lock()
		r.sipAudio = true
		r.mu.Unlock()
		return
	}
	s.sipPending[callID] = true
}

// ServeSIP 阻塞监听 SIP；未启用时等待取消。
func (s *Service) ServeSIP(ctx context.Context) error {
	if s.sip == nil || !s.sip.enabled {
		<-ctx.Done()
		return nil
	}
	return s.sip.serve(ctx)
}

func (u *sipUA) serve(ctx context.Context) error {
	u.rebuildACL()
	ua, err := sipgo.NewUA(
		sipgo.WithUserAgent(u.cfg.UserAgent),
		sipgo.WithUserAgentHostname(u.cfg.Domain()),
	)
	if err != nil {
		return err
	}
	defer ua.Close()
	host := u.cfg.AdvertiseHost()
	port := u.cfg.ListenPort()
	cli, err := sipgo.NewClient(ua,
		sipgo.WithClientHostname(host),
		sipgo.WithClientPort(port),
		sipgo.WithClientNAT(),
	)
	if err != nil {
		return err
	}
	srv, err := sipgo.NewServer(ua)
	if err != nil {
		return err
	}
	contact := sip.ContactHeader{
		Address: sip.Uri{User: u.cfg.UserAgent, Host: host, Port: port},
	}
	u.cli = cli
	u.dlgCli = sipgo.NewDialogClientCache(cli, contact)
	u.dlgSrv = sipgo.NewDialogServerCache(cli, contact)

	srv.OnInvite(u.onInvite)
	srv.OnAck(u.onAck)
	srv.OnBye(u.onByeReq)
	srv.OnCancel(u.onCancel)
	srv.OnRegister(u.onRegister)
	srv.OnOptions(u.onOptions)
	srv.OnPrack(u.onPrack)
	srv.OnUpdate(u.onUpdate)
	srv.OnNoRoute(u.onNoRoute)

	go u.registerLoop(ctx)
	go u.optionsLoop(ctx)

	slog.Info("SIP 已监听", "addr", u.cfg.Listen, "transport", u.cfg.Transport, "external_ip", host)
	if strings.EqualFold(u.cfg.Transport, "tls") {
		cert, err := tls.LoadX509KeyPair(u.cfg.TLSCertFile, u.cfg.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("加载 SIP TLS 证书: %w", err)
		}
		return srv.ListenAndServeTLS(ctx, "tcp", u.cfg.Listen, &tls.Config{Certificates: []tls.Certificate{cert}})
	}
	return srv.ListenAndServe(ctx, "udp", u.cfg.Listen)
}

func (u *sipUA) rebuildACL() {
	nets := make([]*net.IPNet, 0, 8)
	for _, t := range u.cfg.Trunks {
		nets = append(nets, parseAllowedNets(t.AllowedCIDRs)...)
		host := strings.TrimSpace(t.Host)
		if host == "" {
			continue
		}
		if ip := net.ParseIP(host); ip != nil {
			if n := ipNetFromIP(ip); n != nil {
				nets = append(nets, n)
			}
			continue
		}
		ips, err := u.lookupHost(host)
		if err != nil {
			slog.Warn("SIP 中继主机解析失败", "host", host, "err", err)
			continue
		}
		for _, ip := range ips {
			if n := ipNetFromIP(ip); n != nil {
				nets = append(nets, n)
			}
		}
	}
	u.mu.Lock()
	u.allowed = nets
	u.mu.Unlock()
}

func (u *sipUA) ipAllowed(src string) bool {
	ip := hostPortIP(src)
	u.mu.Lock()
	nets := u.allowed
	u.mu.Unlock()
	if len(nets) == 0 {
		return false
	}
	return ipInNets(ip, nets)
}

func (u *sipUA) trunkID(src string) string {
	ip := hostPortIP(src)
	matched := ""
	for _, trunk := range u.cfg.Trunks {
		found := ipInNets(ip, parseAllowedNets(trunk.AllowedCIDRs))
		if !found {
			ips, err := u.lookupHost(trunk.Host)
			if err == nil {
				for _, hostIP := range ips {
					if hostIP.Equal(ip) {
						found = true
						break
					}
				}
			}
		}
		if found {
			if matched != "" {
				return ""
			}
			matched = trunk.ID
		}
	}
	return matched
}

// onInvite 校验来源和设备身份，协商 G.711 媒体并创建呼入 SIP 对话。
func (u *sipUA) onInvite(req *sip.Request, tx sip.ServerTransaction) {
	logSIP("receive", req, "")
	src := req.Source()
	deviceCall := false
	fromUser := ""
	if req.From() != nil {
		fromUser = req.From().Address.User
	}
	if u.device(fromUser, src) != nil {
		if u.dialogByReq(req) == nil && !u.checkRegistrarAuth(req, tx, fromUser) {
			return
		}
		deviceCall = true
	}
	if !deviceCall && (!u.ipAllowed(src) || u.trunkID(src) == "") {
		slog.Warn("拒绝未授权 SIP INVITE", "src", src)
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil))
		return
	}

	// 已存在的对话走重协商；带 To tag 却找不到对话的请求必须拒绝。
	if existing := u.dialogByReq(req); existing != nil {
		if !u.trustedSource(existing, src) {
			_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil))
			return
		}
		u.handleReInvite(existing, req, tx)
		return
	}
	if to := req.To(); to != nil {
		if _, ok := to.Params.Get("tag"); ok {
			res := sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, "Call/Transaction Does Not Exist", nil)
			res.AppendHeader(sipAllowHeader())
			_ = tx.Respond(res)
			return
		}
	}

	dlg, err := u.dlgSrv.ReadInvite(req, tx)
	if err != nil {
		slog.Warn("SIP ReadInvite 失败", "err", err)
		_ = tx.Respond(sip.NewResponseFromRequest(req, 400, "Bad Request", nil))
		return
	}
	_ = dlg.Respond(sip.StatusTrying, "Trying", nil)

	offer := parseSDP(string(req.Body()))
	logSDP("receive", "offer", callIDFromMessage(req), string(req.Body()))
	if !offer.hasG711() {
		_ = dlg.Respond(sip.StatusNotAcceptableHere, "Not Acceptable Here", nil)
		return
	}

	did := req.Recipient.User
	if did == "" && req.To() != nil {
		did = req.To().Address.User
	}
	from := ""
	if req.From() != nil {
		from = req.From().Address.User
	}
	handler := u.onIn
	if deviceCall {
		handler = u.onDevice
	}
	if handler == nil {
		_ = dlg.Respond(sip.StatusTemporarilyUnavailable, "Temporarily Unavailable", nil)
		return
	}

	callID := uuid.New().String()
	u.media.enableSIPAudio(callID)
	rtpSess, err := u.listenRTP()
	if err != nil {
		_ = dlg.Respond(sip.StatusInternalServerError, "Server Internal Error", nil)
		return
	}
	applyRemoteSDP(rtpSess, offer)
	u.media.attachSIPRTP(callID, rtpSess)
	go u.media.sipReadLoop(callID, rtpSess)

	inviteCSeq := uint32(0)
	if cseq := req.CSeq(); cseq != nil {
		inviteCSeq = cseq.SeqNo
	}
	sess := &sipSession{
		callID:     callID,
		sipCallID:  headerCallID(req),
		dlgID:      dlg.ID,
		peerIP:     hostPortIP(src),
		rtp:        rtpSess,
		server:     dlg,
		inviteCSeq: inviteCSeq,
		prackCh:    make(chan struct{}, 1),
	}
	u.putDialog(sess)

	sourceID := u.trunkID(src)
	if deviceCall {
		sourceID = ""
	}
	pendingID, answered := callID, false
	u.media.markSIPAnswerPending(pendingID)
	defer func() { u.media.finishSIPAnswer(pendingID, answered) }()
	got, legID, err := handler(context.Background(), sourceID, did, from, callID)
	rtpSess.mu.Lock()
	rtpSess.legID = legID
	rtpSess.mu.Unlock()
	if err != nil || got == "" || got != callID {
		slog.Warn("SIP INVITE 呼入失败", "did", did, "from", from, "err", err)
		_ = dlg.Respond(sip.StatusNotFound, "Not Found", nil)
		u.dropDialog(sess)
		rtpSess.close()
		_ = u.media.CloseRoom(context.Background(), callID)
		return
	}
	if got != callID {
		callID = got
		u.media.enableSIPAudio(callID)
		u.media.attachSIPRTP(callID, rtpSess)
	}

	sess.mu.Lock()
	ended := sess.ended
	sess.mu.Unlock()
	if ended {
		_ = dlg.Respond(487, "Request Terminated", nil)
		return
	}
	want100rel := require100rel(req.GetHeaders("Require"))
	if want100rel {
		sess.rseq = 1
		if err := u.sendReliableRinging(dlg, sess.rseq); err != nil {
			slog.Warn("SIP 可靠 180 失败", "call_id", callID, "err", err)
			rtpSess.close()
			u.dropDialog(sess)
			u.endCall(callID, false)
			if u.onBye != nil {
				u.onBye(context.Background(), callID)
			}
			return
		}
		if !u.waitPRACK(sess, dlg) {
			_ = dlg.Respond(sip.StatusInternalServerError, "Server Internal Error", nil)
			u.endCall(callID, false)
			if u.onBye != nil {
				u.onBye(context.Background(), callID)
			}
			return
		}
	} else {
		_ = dlg.Respond(sip.StatusRinging, "Ringing", nil)
	}

	sdp := buildAnswerSDP(u.cfg.AdvertiseHost(), rtpSess.localPort(), offer)
	if sdp == "" {
		_ = dlg.Respond(sip.StatusNotAcceptableHere, "Not Acceptable Here", nil)
		u.endCall(callID, false)
		if u.onBye != nil {
			u.onBye(context.Background(), callID)
		}
		return
	}
	hdrs := u.answerHeaders(req)
	logSDP("send", "answer", callID, sdp)
	if err := dlg.Respond(sip.StatusOK, "OK", []byte(sdp), hdrs...); err != nil {
		slog.Warn("SIP 200 SDP 失败", "call_id", callID, "err", err)
		u.endCall(callID, false)
		if u.onBye != nil {
			u.onBye(context.Background(), callID)
		}
		return
	}
	u.armSessionTimerFrom(sess, headerValue(req, "Session-Expires"), "uas")
	sess.mu.Lock()
	sess.confirmed = true
	sess.mu.Unlock()
	answered = true
	slog.Info("SIP 200 OK", "call_id", callID, "did", did, "from", from, "sip_call_id", sess.sipCallID)
}

// handleReInvite 在已有对话中校验 CSeq 并更新媒体协商与会话计时。
func (u *sipUA) handleReInvite(d *sipSession, req *sip.Request, tx sip.ServerTransaction) {
	logSIP("receive", req, d.callID)
	d.mu.Lock()
	ended, confirmed, rtpSess, lastCSeq := d.ended, d.confirmed, d.rtp, d.inviteCSeq
	d.mu.Unlock()
	if ended {
		res := sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, "Call/Transaction Does Not Exist", nil)
		res.AppendHeader(sipAllowHeader())
		_ = tx.Respond(res)
		return
	}
	cseq := uint32(0)
	if h := req.CSeq(); h != nil {
		cseq = h.SeqNo
	}
	if cseq < lastCSeq {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusInternalServerError, "Server Internal Error", nil))
		return
	}
	if cseq > lastCSeq {
		d.mu.Lock()
		d.inviteCSeq = cseq
		d.mu.Unlock()
		if len(req.Body()) > 0 {
			offer := parseSDP(string(req.Body()))
			applyRemoteSDP(rtpSess, offer)
			if confirmed && rtpSess != nil {
				sdp := buildAnswerSDP(u.cfg.AdvertiseHost(), rtpSess.localPort(), offer)
				if sdp == "" {
					_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotAcceptableHere, "Not Acceptable Here", nil))
					return
				}
				res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", []byte(sdp))
				res.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
				res.AppendHeader(sipAllowHeader())
				_ = tx.Respond(res)
				return
			}
		}
	}
	if confirmed && rtpSess != nil {
		sdp := buildAnswerSDP(u.cfg.AdvertiseHost(), rtpSess.localPort(), sdpMedia{Types: []int{int(rtpSess.currentPT()), 101}})
		res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", []byte(sdp))
		res.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
		res.AppendHeader(sipAllowHeader())
		_ = tx.Respond(res)
		return
	}
	_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusRinging, "Ringing", nil))
}

func (u *sipUA) sendReliableRinging(dlg *sipgo.DialogServerSession, rseq uint32) error {
	return dlg.Respond(sip.StatusRinging, "Ringing", nil,
		sip.NewHeader("Require", "100rel"),
		sip.NewHeader("RSeq", strconv.FormatUint(uint64(rseq), 10)),
		sipAllowHeader(),
	)
}

func (u *sipUA) waitPRACK(sess *sipSession, dlg *sipgo.DialogServerSession) bool {
	deadline := time.Now().Add(sipPRACKWait)
	wait := 500 * time.Millisecond
	for time.Now().Before(deadline) {
		sess.mu.Lock()
		ended := sess.ended
		rseq := sess.rseq
		sess.mu.Unlock()
		if ended {
			return false
		}
		select {
		case <-sess.prackCh:
			return true
		case <-dlg.Context().Done():
			return false
		case <-time.After(wait):
			_ = u.sendReliableRinging(dlg, rseq)
			wait *= 2
			if wait > 4*time.Second {
				wait = 4 * time.Second
			}
		}
	}
	return false
}

func (u *sipUA) answerHeaders(req *sip.Request) []sip.Header {
	hdrs := []sip.Header{
		sip.NewHeader("Content-Type", "application/sdp"),
		sipAllowHeader(),
		sip.NewHeader("Supported", "100rel, timer"),
	}
	if se := headerValue(req, "Session-Expires"); se != "" {
		sec, refresher := parseSessionExpires(se)
		if sec < sipSessionMinSE {
			sec = sipSessionMinSE
		}
		if refresher == "" {
			refresher = "uas"
		}
		hdrs = append(hdrs, sip.NewHeader("Session-Expires", strconv.Itoa(sec)+";refresher="+refresher))
	} else if u.cfg.SessionExpiresSec >= sipSessionMinSE && headerHasToken(req.GetHeaders("Supported"), "timer") {
		hdrs = append(hdrs, sip.NewHeader("Session-Expires", strconv.Itoa(u.cfg.SessionExpiresSec)+";refresher=uas"))
	}
	return hdrs
}

func (u *sipUA) onAck(req *sip.Request, tx sip.ServerTransaction) {
	logSIP("receive", req, "")
	if u.dlgSrv != nil {
		_ = u.dlgSrv.ReadAck(req, tx)
	}
	d := u.dialogByReq(req)
	if d == nil {
		d = u.dialogBySIP(headerCallID(req))
	}
	if d == nil || !u.trustedSource(d, req.Source()) {
		return
	}
	d.mu.Lock()
	rtpSess := d.rtp
	d.mu.Unlock()
	if rtpSess != nil && len(req.Body()) > 0 {
		applyRemoteSDP(rtpSess, parseSDP(string(req.Body())))
	}
}

func (u *sipUA) onByeReq(req *sip.Request, tx sip.ServerTransaction) {
	logSIP("receive", req, "")
	d := u.inDialog(req)
	if d == nil {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, "Call/Transaction Does Not Exist", nil))
		return
	}
	if !u.trustedSource(d, req.Source()) {
		slog.Warn("拒绝来源不符的 SIP BYE", "src", req.Source(), "call_id", d.callID)
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil))
		return
	}
	if u.dlgSrv != nil {
		if err := u.dlgSrv.ReadBye(req, tx); err != nil && u.dlgCli != nil {
			_ = u.dlgCli.ReadBye(req, tx)
		}
	} else if u.dlgCli != nil {
		_ = u.dlgCli.ReadBye(req, tx)
	}
	callID := d.callID
	u.dropDialog(d)
	u.endDialog(d, false)
	if u.onBye != nil {
		u.onBye(context.Background(), callID)
	}
}

func (u *sipUA) onCancel(req *sip.Request, tx sip.ServerTransaction) {
	logSIP("receive", req, "")
	d := u.dialogBySIP(headerCallID(req))
	if d == nil || !u.cancelMatches(d, req) {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, "Call/Transaction Does Not Exist", nil))
		return
	}
	if !u.trustedSource(d, req.Source()) {
		slog.Warn("拒绝来源不符的 SIP CANCEL", "src", req.Source(), "call_id", d.callID)
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil))
		return
	}
	_ = tx.Respond(sip.NewResponseFromRequest(req, 200, "OK", nil))
	callID := d.callID
	u.dropDialog(d)
	u.endDialog(d, false)
	if u.onBye != nil {
		u.onBye(context.Background(), callID)
	}
}

func (u *sipUA) onPrack(req *sip.Request, tx sip.ServerTransaction) {
	logSIP("receive", req, "")
	d := u.dialogByReq(req)
	if d == nil {
		d = u.dialogBySIP(headerCallID(req))
	}
	if d == nil {
		res := sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, "Call/Transaction Does Not Exist", nil)
		res.AppendHeader(sipAllowHeader())
		_ = tx.Respond(res)
		return
	}
	if !u.trustedSource(d, req.Source()) {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil))
		return
	}
	rseq, cseq, method := parseRAck(headerValue(req, "RAck"))
	d.mu.Lock()
	ok := rseq == d.rseq && cseq == d.inviteCSeq && method == "INVITE"
	ch := d.prackCh
	d.mu.Unlock()
	if !ok {
		_ = tx.Respond(sip.NewResponseFromRequest(req, 481, "Call/Transaction Does Not Exist", nil))
		return
	}
	_ = tx.Respond(sip.NewResponseFromRequest(req, 200, "OK", nil))
	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (u *sipUA) onUpdate(req *sip.Request, tx sip.ServerTransaction) {
	logSIP("receive", req, "")
	d := u.inDialog(req)
	if d == nil {
		res := sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, "Call/Transaction Does Not Exist", nil)
		res.AppendHeader(sipAllowHeader())
		_ = tx.Respond(res)
		return
	}
	if !u.trustedSource(d, req.Source()) {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil))
		return
	}
	d.mu.Lock()
	rtpSess, confirmed := d.rtp, d.confirmed
	d.mu.Unlock()
	var body []byte
	if confirmed && rtpSess != nil && len(req.Body()) > 0 {
		offer := parseSDP(string(req.Body()))
		applyRemoteSDP(rtpSess, offer)
		sdp := buildAnswerSDP(u.cfg.AdvertiseHost(), rtpSess.localPort(), offer)
		body = []byte(sdp)
	}
	res := sip.NewResponseFromRequest(req, 200, "OK", body)
	res.AppendHeader(sipAllowHeader())
	if len(body) > 0 {
		res.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
	}
	if se := headerValue(req, "Session-Expires"); se != "" {
		res.AppendHeader(sip.NewHeader("Session-Expires", se))
	}
	_ = tx.Respond(res)
}

func (u *sipUA) onNoRoute(req *sip.Request, tx sip.ServerTransaction) {
	logSIP("receive", req, "")
	res := sip.NewResponseFromRequest(req, sip.StatusMethodNotAllowed, "Method Not Allowed", nil)
	res.AppendHeader(sipAllowHeader())
	_ = tx.Respond(res)
}

func (u *sipUA) onOptions(req *sip.Request, tx sip.ServerTransaction) {
	logSIP("receive", req, "")
	res := sip.NewResponseFromRequest(req, 200, "OK", nil)
	res.AppendHeader(sipAllowHeader())
	res.AppendHeader(sip.NewHeader("Supported", "100rel, timer"))
	_ = tx.Respond(res)
}

func sipMessageAbsent(msg sip.Message) bool {
	if msg == nil {
		return true
	}
	v := reflect.ValueOf(msg)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

func headerCallID(msg sip.Message) string {
	if sipMessageAbsent(msg) {
		return ""
	}
	if h := msg.CallID(); h != nil {
		return h.Value()
	}
	return ""
}

func callIDFromMessage(msg sip.Message) string { return headerCallID(msg) }

func logSIP(direction string, msg sip.Message, callID string) {
	if sipMessageAbsent(msg) {
		return
	}
	if callID == "" {
		callID = headerCallID(msg)
	}
	ctx := observability.WithFields(context.Background(), observability.Fields{CallID: callID})
	observability.Event(ctx, "sip", "sip.message", direction, "ok", "", time.Time{},
		"direction", direction, "sip_call_id", headerCallID(msg), "body", observability.Redact(msg.String()))
}

func logSDP(direction, kind, callID, body string) {
	ctx := observability.WithFields(context.Background(), observability.Fields{CallID: callID})
	observability.Event(ctx, "sdp", "sdp."+kind, direction, "ok", "", time.Time{},
		"direction", direction, "body", observability.Redact(body))
}

func headerValue(msg sip.Message, name string) string {
	if msg == nil {
		return ""
	}
	hs := msg.GetHeaders(name)
	if len(hs) == 0 {
		return ""
	}
	return hs[0].Value()
}

func (s *Service) SetDeviceHandler(h InboundSIPHandler) {
	if s.sip != nil {
		s.sip.onDevice = h
	}
}

func (s *Service) PrepareSIP(callID string) { s.enableSIPAudio(callID) }
