package media

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/google/uuid"
	"github.com/icholy/digest"

	"open-switch/internal/config"
	"open-switch/internal/errs"
)

const (
	sipSessionMinSE       = 90
	sipMaxRedirectHops    = 2
	sipPRACKWait          = 32 * time.Second
	sipRegisterMinExpires = 60
)

// InboundSIPHandler 由组合根注入：DID 呼入时创建 L3 通话。callID 为预分配 ID。
type InboundSIPHandler func(ctx context.Context, did, from, callID string) (gotCallID, legID string, err error)

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
	nonces   map[string]time.Time
	allowed  []*net.IPNet
	cli      *sipgo.Client
	dlgCli   *sipgo.DialogClientCache
	dlgSrv   *sipgo.DialogServerCache
}

type sipSession struct {
	mu          sync.Mutex
	callID      string
	sipCallID   string
	dlgID       string
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
		ips, err := net.LookupIP(host)
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

func (u *sipUA) onInvite(req *sip.Request, tx sip.ServerTransaction) {
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
	if !deviceCall && !u.ipAllowed(src) {
		slog.Warn("拒绝未授权 SIP INVITE", "src", src)
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil))
		return
	}

	if existing := u.dialogByReq(req); existing != nil {
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
		rtp:        rtpSess,
		server:     dlg,
		inviteCSeq: inviteCSeq,
		prackCh:    make(chan struct{}, 1),
	}
	u.putDialog(sess)

	got, legID, err := handler(context.Background(), did, from, callID)
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
		return
	}
	hdrs := u.answerHeaders(req)
	if err := dlg.Respond(sip.StatusOK, "OK", []byte(sdp), hdrs...); err != nil {
		slog.Warn("SIP 200 SDP 失败", "call_id", callID, "err", err)
		u.endCall(callID, false)
		return
	}
	u.armSessionTimerFrom(sess, headerValue(req, "Session-Expires"), "uas")
	sess.mu.Lock()
	sess.confirmed = true
	sess.mu.Unlock()
	slog.Info("SIP 200 OK", "call_id", callID, "did", did, "from", from, "sip_call_id", sess.sipCallID)
}

func (u *sipUA) handleReInvite(d *sipSession, req *sip.Request, tx sip.ServerTransaction) {
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
	if u.dlgSrv != nil {
		_ = u.dlgSrv.ReadAck(req, tx)
	}
	d := u.dialogByReq(req)
	if d == nil {
		d = u.dialogBySIP(headerCallID(req))
	}
	if d == nil {
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
	d := u.dialogByReq(req)
	if d == nil {
		d = u.dialogBySIP(headerCallID(req))
	}
	if u.dlgSrv != nil {
		if err := u.dlgSrv.ReadBye(req, tx); err != nil && u.dlgCli != nil {
			_ = u.dlgCli.ReadBye(req, tx)
		}
	} else if u.dlgCli != nil {
		_ = u.dlgCli.ReadBye(req, tx)
	}
	if d == nil {
		return
	}
	callID := d.callID
	u.dropDialog(d)
	u.endDialog(d, false)
	if u.onBye != nil {
		u.onBye(context.Background(), callID)
	}
}

func (u *sipUA) onCancel(req *sip.Request, tx sip.ServerTransaction) {
	_ = tx.Respond(sip.NewResponseFromRequest(req, 200, "OK", nil))
	d := u.dialogBySIP(headerCallID(req))
	if d == nil {
		return
	}
	callID := d.callID
	u.dropDialog(d)
	u.endDialog(d, false)
	if u.onBye != nil {
		u.onBye(context.Background(), callID)
	}
}

func (u *sipUA) onPrack(req *sip.Request, tx sip.ServerTransaction) {
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
	d := u.dialogByReq(req)
	if d == nil {
		res := sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, "Call/Transaction Does Not Exist", nil)
		res.AppendHeader(sipAllowHeader())
		_ = tx.Respond(res)
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
	res := sip.NewResponseFromRequest(req, sip.StatusMethodNotAllowed, "Method Not Allowed", nil)
	res.AppendHeader(sipAllowHeader())
	_ = tx.Respond(res)
}

func (u *sipUA) onOptions(req *sip.Request, tx sip.ServerTransaction) {
	res := sip.NewResponseFromRequest(req, 200, "OK", nil)
	res.AppendHeader(sipAllowHeader())
	res.AppendHeader(sip.NewHeader("Supported", "100rel, timer"))
	_ = tx.Respond(res)
}

func (u *sipUA) onRegister(req *sip.Request, tx sip.ServerTransaction) {
	src := req.Source()
	if !u.cfg.LocalRegistrar {
		slog.Warn("拒绝 SIP REGISTER", "src", src, "local_registrar", u.cfg.LocalRegistrar)
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil))
		return
	}
	aor := ""
	if req.To() != nil {
		aor = req.To().Address.User
	}
	if aor == "" && req.From() != nil {
		aor = req.From().Address.User
	}
	if aor == "" {
		_ = tx.Respond(sip.NewResponseFromRequest(req, 400, "Bad Request", nil))
		return
	}
	if !u.checkRegistrarAuth(req, tx, aor) {
		return
	}

	callID := headerCallID(req)
	cseq := uint32(0)
	if h := req.CSeq(); h != nil {
		cseq = h.SeqNo
	}
	u.mu.Lock()
	if prev, ok := u.maxBindingCSeq(aor, callID); ok && cseq <= prev {
		u.mu.Unlock()
		_ = tx.Respond(sip.NewResponseFromRequest(req, 400, "Bad Request", nil))
		return
	}
	u.mu.Unlock()

	expires, brief := registerExpires(req)
	if brief {
		res := sip.NewResponseFromRequest(req, sip.StatusIntervalToBrief, "Interval Too Brief", nil)
		res.AppendHeader(sip.NewHeader("Min-Expires", strconv.Itoa(sipRegisterMinExpires)))
		_ = tx.Respond(res)
		return
	}

	cont := req.Contact()
	if cont != nil && cont.Address.Wildcard && expires != 0 {
		_ = tx.Respond(sip.NewResponseFromRequest(req, 400, "Bad Request", nil))
		return
	}

	u.mu.Lock()
	u.purgeExpiredBindingsLocked()
	if cont != nil && cont.Address.Wildcard {
		u.removeBindingsByCallIDLocked(aor, callID)
	} else if cont != nil {
		addr := registerContactAddr(src, cont)
		if expires == 0 {
			u.removeBindingLocked(aor, cont.Address)
		} else {
			cp := cont.Address.Clone()
			b := sipBinding{
				AOR:       aor,
				ExpiresAt: time.Now().Add(time.Duration(expires) * time.Second),
				CallID:    callID,
				CSeq:      cseq,
				Addr:      addr,
			}
			if cp != nil {
				b.Contact = *cp
			} else {
				b.Contact = cont.Address
			}
			u.upsertBindingLocked(b)
		}
	}
	contacts := append([]sipBinding(nil), u.binds[aor]...)
	u.mu.Unlock()

	res := sip.NewResponseFromRequest(req, 200, "OK", nil)
	res.AppendHeader(sipAllowHeader())
	now := time.Now()
	for _, b := range contacts {
		rem := int(b.ExpiresAt.Sub(now).Seconds())
		if rem < 0 {
			continue
		}
		ch := &sip.ContactHeader{Address: b.Contact, Params: sip.NewParams()}
		ch.Params.Add("expires", strconv.Itoa(rem))
		res.AppendHeader(ch)
	}
	res.AppendHeader(sip.NewHeader("Expires", strconv.Itoa(expires)))
	_ = tx.Respond(res)
	slog.Info("SIP REGISTER", "user", aor, "src", src, "expires", expires)
}

func (u *sipUA) checkRegistrarAuth(req *sip.Request, tx sip.ServerTransaction, aor string) bool {
	pass := ""
	if d := u.device(aor, req.Source()); d != nil {
		pass = d.Password
	}
	auth := req.GetHeader("Authorization")
	val := ""
	if auth != nil {
		val = auth.Value()
	}
	cred, parseErr := digest.ParseCredentials(val)
	ok := parseErr == nil && cred.Realm == u.cfg.Domain() && cred.URI == req.Recipient.String() && cred.QOP == "auth" && verifyRegistrarDigest(val, string(req.Method), aor, pass, u.nonceValid)
	if ok {
		u.mu.Lock()
		delete(u.nonces, cred.Nonce)
		u.mu.Unlock()
		return true
	}
	stale := val != ""
	nonce := newDigestNonce()
	u.mu.Lock()
	for n, exp := range u.nonces {
		if time.Now().After(exp) {
			delete(u.nonces, n)
		}
	}
	if len(u.nonces) >= 4096 {
		u.mu.Unlock()
		_ = tx.Respond(sip.NewResponseFromRequest(req, 503, "Unavailable", nil))
		return false
	}
	u.nonces[nonce] = time.Now().Add(5 * time.Minute)
	u.mu.Unlock()
	chal := registrarChallenge(u.cfg.Domain(), nonce, stale)
	res := sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Unauthorized", nil)
	res.AppendHeader(sip.NewHeader("WWW-Authenticate", chal.String()))
	_ = tx.Respond(res)
	return false
}

func (u *sipUA) nonceValid(nonce string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	exp, ok := u.nonces[nonce]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(u.nonces, nonce)
		return false
	}
	return true
}

func registerExpires(req *sip.Request) (expires int, tooBrief bool) {
	expires = 300
	if h := req.GetHeader("Expires"); h != nil {
		if n, err := strconv.Atoi(strings.TrimSpace(h.Value())); err == nil {
			expires = n
		}
	}
	if cont := req.Contact(); cont != nil && cont.Params != nil {
		if v, ok := cont.Params.Get("expires"); ok {
			if n, err := strconv.Atoi(v); err == nil {
				expires = n
			}
		}
	}
	if expires < 0 {
		expires = 0
	}
	if expires > 3600 {
		expires = 3600
	}
	if expires > 0 && expires < sipRegisterMinExpires {
		return expires, true
	}
	return expires, false
}

func registerContactAddr(src string, cont *sip.ContactHeader) *net.UDPAddr {
	bind := hostPortIP(src)
	port := 5060
	if _, p, err := net.SplitHostPort(src); err == nil {
		port, _ = strconv.Atoi(p)
	}
	if cont != nil {
		if cont.Address.Host != "" {
			if ip := net.ParseIP(cont.Address.Host); ip != nil && !ip.IsUnspecified() {
				bind = ip
			}
		}
		if cont.Address.Port > 0 {
			port = cont.Address.Port
		}
	}
	return &net.UDPAddr{IP: bind, Port: port}
}

func (u *sipUA) maxBindingCSeq(aor, callID string) (uint32, bool) {
	var max uint32
	found := false
	for _, b := range u.binds[aor] {
		if b.CallID != callID {
			continue
		}
		found = true
		if b.CSeq > max {
			max = b.CSeq
		}
	}
	return max, found
}

func (u *sipUA) upsertBindingLocked(b sipBinding) {
	list := u.binds[b.AOR]
	for i := range list {
		if uriEqual(list[i].Contact, b.Contact) {
			// A refreshed contact becomes the preferred target. Otherwise a stale
			// first registration can keep receiving every ACD offer.
			updated := append([]sipBinding{b}, list[:i]...)
			u.binds[b.AOR] = append(updated, list[i+1:]...)
			return
		}
	}
	u.binds[b.AOR] = append([]sipBinding{b}, list...)
}

func (u *sipUA) removeBindingLocked(aor string, contact sip.Uri) {
	list := u.binds[aor]
	out := list[:0]
	for _, b := range list {
		if !uriEqual(b.Contact, contact) {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		delete(u.binds, aor)
		return
	}
	u.binds[aor] = out
}

func (u *sipUA) removeBindingsByCallIDLocked(aor, callID string) {
	list := u.binds[aor]
	out := list[:0]
	for _, b := range list {
		if b.CallID != callID {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		delete(u.binds, aor)
		return
	}
	u.binds[aor] = out
}

func (u *sipUA) purgeExpiredBindingsLocked() {
	now := time.Now()
	for aor, list := range u.binds {
		out := list[:0]
		for _, b := range list {
			if b.ExpiresAt.After(now) {
				out = append(out, b)
			}
		}
		if len(out) == 0 {
			delete(u.binds, aor)
		} else {
			u.binds[aor] = out
		}
	}
	for n, exp := range u.nonces {
		if now.After(exp) {
			delete(u.nonces, n)
		}
	}
}

func uriEqual(a, b sip.Uri) bool {
	return strings.EqualFold(a.User, b.User) && strings.EqualFold(a.Host, b.Host) && a.Port == b.Port
}

func (u *sipUA) originate(ctx context.Context, callID, legID, dial, trunkID string) error {
	tr := u.pickTrunk(trunkID)
	if trunkID == "@device" {
		tr = nil
		if u.lookupReg(dial) == nil {
			return errs.Unprocessable("SIP 坐席未注册", errs.CodeSIPDisabled)
		}
	}
	if trunkID != "" && trunkID != "@device" && tr == nil {
		return errs.InvalidRequest("中继不存在")
	}
	if u.lookupReg(dial) != nil && trunkID == "" {
		tr = nil
	}
	if tr == nil && u.lookupReg(dial) == nil {
		return errs.Unprocessable("未找到 SIP 中继或已注册分机", errs.CodeSIPDisabled)
	}
	if tr != nil {
		dial = tr.NormalizeDial(dial)
	}
	u.media.enableSIPAudio(callID)
	rtpSess, err := u.listenRTP()
	if err != nil {
		return err
	}
	rtpSess.legID = legID
	u.media.attachSIPRTP(callID, rtpSess)
	go u.media.sipReadLoop(callID, rtpSess)

	recipient, cliUser, codecs, user, pass := u.outboundTarget(dial, tr)
	sdp := buildAudioSDP(u.cfg.AdvertiseHost(), rtpSess.localPort(), codecs)
	if u.dlgCli == nil {
		rtpSess.close()
		return errs.Unprocessable("SIP 未就绪", errs.CodeSIPDisabled)
	}

	se := u.cfg.SessionExpiresSec
	for hop := 0; hop <= sipMaxRedirectHops; hop++ {
		headers := u.inviteHeaders(cliUser, tr, se)
		headers = append(headers, sip.NewHeader("Content-Type", "application/sdp"))
		dlg, err := u.dlgCli.Invite(ctx, recipient, []byte(sdp), headers...)
		if err != nil {
			rtpSess.close()
			return err
		}
		sipCID := headerCallID(dlg.InviteRequest)
		inviteCSeq := uint32(0)
		if dlg.InviteRequest != nil && dlg.InviteRequest.CSeq() != nil {
			inviteCSeq = dlg.InviteRequest.CSeq().SeqNo
		}
		waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		sess := &sipSession{
			cancel:     cancel,
			callID:     callID,
			sipCallID:  sipCID,
			rtp:        rtpSess,
			client:     dlg,
			inviteCSeq: inviteCSeq,
			prackCh:    make(chan struct{}, 1),
		}
		u.putDialog(sess)

		err = dlg.WaitAnswer(waitCtx, sipgo.AnswerOptions{
			Username: user,
			Password: pass,
			OnResponse: func(res *sip.Response) error {
				if res == nil {
					return nil
				}
				if res.StatusCode > 100 && res.StatusCode < 200 && require100rel(res.GetHeaders("Require")) {
					if perr := u.sendPRACK(waitCtx, dlg, res); perr != nil {
						slog.Warn("SIP PRACK 失败", "call_id", callID, "err", perr)
						return perr
					}
				}
				if (res.StatusCode == 183 || res.StatusCode == 180 || res.IsSuccess()) && len(res.Body()) > 0 {
					applyRemoteSDP(rtpSess, parseSDP(string(res.Body())))
				}
				return nil
			},
		})
		cancel()
		if err == nil {
			if err := dlg.Ack(ctx); err != nil {
				slog.Warn("SIP ACK 失败", "call_id", callID, "err", err)
			}
			sess.dlgID = dlg.ID
			u.putDialog(sess)
			if dlg.InviteResponse != nil {
				u.armSessionTimerFrom(sess, headerValue(dlg.InviteResponse, "Session-Expires"), "uac")
			}
			sess.mu.Lock()
			sess.confirmed = true
			sess.mu.Unlock()
			slog.Info("SIP INVITE 出局已接通", "call_id", callID, "dial", dial, "sip_call_id", sipCID, "trunk", trunkID)
			return nil
		}

		_ = dlg.Close()
		u.dropDialog(sess)

		var dres *sipgo.ErrDialogResponse
		if !errors.As(err, &dres) || dres == nil || dres.Res == nil {
			rtpSess.close()
			return errs.Unprocessable("SIP 对端拒绝或超时", errs.CodeSIPDisabled)
		}
		code := dres.Res.StatusCode
		if code >= 300 && code < 400 {
			if uri, ok := redirectURI(dres.Res); ok && strings.EqualFold(uri.Host, recipient.Host) && uri.Port == recipient.Port {
				recipient = uri
				slog.Info("SIP 跟随 3xx", "call_id", callID, "status", code, "hop", hop+1)
				continue
			}
		}
		if code == 422 {
			minSE := parseMinSE(headerValue(dres.Res, "Min-SE"))
			if minSE < sipSessionMinSE {
				minSE = sipSessionMinSE
			}
			if minSE > se {
				se = minSE
				slog.Info("SIP 422 提升 Session-Expires", "call_id", callID, "session_expires", se)
				continue
			}
		}
		rtpSess.close()
		return errs.Unprocessable("SIP 对端拒绝或超时", errs.CodeSIPDisabled)
	}
	rtpSess.close()
	return errs.Unprocessable("SIP 对端拒绝或超时", errs.CodeSIPDisabled)
}

func (u *sipUA) sendPRACK(ctx context.Context, dlg *sipgo.DialogClientSession, res *sip.Response) error {
	if dlg == nil || res == nil {
		return nil
	}
	rseq := strings.TrimSpace(headerValue(res, "RSeq"))
	if rseq == "" {
		return fmt.Errorf("100rel 缺少 RSeq")
	}
	invCSeq := uint32(1)
	if dlg.InviteRequest != nil && dlg.InviteRequest.CSeq() != nil {
		invCSeq = dlg.InviteRequest.CSeq().SeqNo
	}
	dest := dlg.InviteRequest.Recipient
	if res.Contact() != nil {
		dest = res.Contact().Address
	}
	req := sip.NewRequest(sip.PRACK, dest)
	req.AppendHeader(sip.NewHeader("RAck", fmt.Sprintf("%s %d INVITE", rseq, invCSeq)))
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pr, err := dlg.Do(pctx, req)
	if err != nil {
		return err
	}
	if pr != nil && pr.StatusCode >= 300 {
		return fmt.Errorf("PRACK 被拒 %d", pr.StatusCode)
	}
	return nil
}

func (u *sipUA) inviteHeaders(cliUser string, tr *config.SIPTrunkConfig, se int) []sip.Header {
	domain := u.cfg.Domain()
	from := &sip.FromHeader{
		Address: sip.Uri{User: cliUser, Host: domain},
		Params:  sip.NewParams(),
	}
	from.Params.Add("tag", sip.GenerateTagN(8))
	contact := &sip.ContactHeader{
		Address: sip.Uri{User: cliUser, Host: u.cfg.AdvertiseHost(), Port: u.cfg.ListenPort()},
	}
	supported := "100rel"
	if se >= sipSessionMinSE {
		supported = "100rel, timer"
	}
	headers := []sip.Header{
		from,
		contact,
		sipAllowHeader(),
		sip.NewHeader("Supported", supported),
		sip.NewHeader("User-Agent", u.cfg.UserAgent),
	}
	// RFC 3325：P-Asserted-Identity 仅发往受信任中继，不发给本机软电话。
	if tr != nil {
		headers = append(headers, sip.NewHeader("P-Asserted-Identity", fmt.Sprintf("<sip:%s@%s>", cliUser, domain)))
	}
	if se >= sipSessionMinSE {
		headers = append(headers,
			sip.NewHeader("Session-Expires", strconv.Itoa(se)+";refresher=uac"),
			sip.NewHeader("Min-SE", strconv.Itoa(sipSessionMinSE)),
		)
	}
	return headers
}

func (u *sipUA) outboundTarget(dial string, tr *config.SIPTrunkConfig) (sip.Uri, string, []string, string, string) {
	cli := u.cfg.UserAgent
	codecs := []string{"PCMU", "PCMA"}
	user, pass := "", ""
	if tr != nil {
		cli = tr.CLIUser(u.cfg.UserAgent)
		codecs = u.offerCodecs(tr)
		user, pass = tr.Username, tr.Password
	}
	if addr := u.lookupReg(dial); addr != nil {
		return sip.Uri{User: dial, Host: addr.IP.String(), Port: addr.Port}, cli, codecs, "", ""
	}
	host, port := "127.0.0.1", 5060
	if tr != nil {
		host, port = tr.Host, tr.Port
	}
	uri := sip.Uri{User: dial, Host: host, Port: port}
	if strings.EqualFold(u.cfg.Transport, "tls") {
		uri.UriParams = sip.NewParams()
		uri.UriParams.Add("transport", "tls")
	}
	return uri, cli, codecs, user, pass
}

func (u *sipUA) offerCodecs(tr *config.SIPTrunkConfig) []string {
	if tr != nil && len(tr.Codecs) > 0 {
		return tr.Codecs
	}
	if len(u.cfg.Trunks) > 0 && len(u.cfg.Trunks[0].Codecs) > 0 {
		return u.cfg.Trunks[0].Codecs
	}
	return []string{"PCMU", "PCMA"}
}

func (u *sipUA) pickTrunk(id string) *config.SIPTrunkConfig {
	if len(u.cfg.Trunks) == 0 {
		return nil
	}
	if id == "" {
		return &u.cfg.Trunks[0]
	}
	for i := range u.cfg.Trunks {
		if u.cfg.Trunks[i].ID == id {
			return &u.cfg.Trunks[i]
		}
	}
	return nil
}

func (u *sipUA) lookupReg(user string) *net.UDPAddr {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.purgeExpiredBindingsLocked()
	list := u.binds[user]
	if len(list) == 0 || list[0].Addr == nil {
		return nil
	}
	return new(*list[0].Addr)
}

func (u *sipUA) registerLoop(ctx context.Context) {
	for i := range u.cfg.Trunks {
		tr := u.cfg.Trunks[i]
		if !tr.Register {
			continue
		}
		go u.registerTrunk(ctx, tr)
	}
}

func (u *sipUA) registerTrunk(ctx context.Context, tr config.SIPTrunkConfig) {
	expire := tr.ExpireSec
	if expire <= 0 {
		expire = 300
	}
	u.doRegister(ctx, tr, expire)
	ticker := time.NewTicker(time.Duration(expire) * 2 / 3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			u.doRegister(context.Background(), tr, 0)
			return
		case <-ticker.C:
			u.doRegister(ctx, tr, expire)
		}
	}
}

func (u *sipUA) doRegister(ctx context.Context, tr config.SIPTrunkConfig, expire int) {
	if u.cli == nil {
		return
	}
	recipient := registerRequestURI(tr.Host, tr.Port)
	req := sip.NewRequest(sip.REGISTER, recipient)
	aor := sip.Uri{User: tr.Username, Host: tr.Host}
	from := &sip.FromHeader{Address: aor, Params: sip.NewParams()}
	from.Params.Add("tag", sip.GenerateTagN(16))
	req.AppendHeader(from)
	req.AppendHeader(&sip.ToHeader{Address: aor})
	contact := fmt.Sprintf("<sip:%s@%s:%d>", tr.CLIUser(u.cfg.UserAgent), u.cfg.AdvertiseHost(), u.cfg.ListenPort())
	req.AppendHeader(sip.NewHeader("Contact", contact))
	req.AppendHeader(sip.NewHeader("Expires", strconv.Itoa(expire)))
	req.AppendHeader(sipAllowHeader())
	if strings.EqualFold(u.cfg.Transport, "tls") {
		req.SetTransport("TLS")
	} else {
		req.SetTransport("UDP")
	}
	tx, err := u.cli.TransactionRequest(ctx, req, sipgo.ClientRequestRegisterBuild)
	if err != nil {
		slog.Error("SIP REGISTER 发送失败", "trunk", tr.ID, "err", err)
		return
	}
	defer tx.Terminate()
	res, err := waitFinal(ctx, tx)
	if err != nil {
		slog.Error("SIP REGISTER 无响应", "trunk", tr.ID, "err", err)
		return
	}
	if res.StatusCode == sip.StatusUnauthorized || res.StatusCode == sip.StatusProxyAuthRequired {
		authHeader := "WWW-Authenticate"
		if res.StatusCode == sip.StatusProxyAuthRequired {
			authHeader = "Proxy-Authenticate"
		}
		h := res.GetHeader(authHeader)
		if h == nil {
			slog.Error("SIP REGISTER 质询缺少头", "trunk", tr.ID)
			return
		}
		digestURI := recipient.Addr()
		cred, err := digestAuthorization(h.Value(), "REGISTER", digestURI, tr.Username, tr.Password, tr.Realm)
		if err != nil {
			slog.Error("SIP REGISTER Digest 失败", "trunk", tr.ID, "err", err)
			return
		}
		newReq := req.Clone()
		newReq.RemoveHeader("Via")
		if res.StatusCode == sip.StatusProxyAuthRequired {
			newReq.RemoveHeader("Proxy-Authorization")
			newReq.AppendHeader(sip.NewHeader("Proxy-Authorization", cred))
		} else {
			newReq.RemoveHeader("Authorization")
			newReq.AppendHeader(sip.NewHeader("Authorization", cred))
		}
		tx2, err := u.cli.TransactionRequest(ctx, newReq, sipgo.ClientRequestIncreaseCSEQ, sipgo.ClientRequestAddVia)
		if err != nil {
			slog.Error("SIP REGISTER 鉴权重试失败", "trunk", tr.ID, "err", err)
			return
		}
		defer tx2.Terminate()
		res, err = waitFinal(ctx, tx2)
		if err != nil {
			slog.Error("SIP REGISTER 鉴权无响应", "trunk", tr.ID, "err", err)
			return
		}
	}
	if res.StatusCode != 200 {
		slog.Error("SIP REGISTER 被拒", "trunk", tr.ID, "status", res.StatusCode)
		return
	}
	if expire == 0 {
		slog.Info("SIP 已注销", "trunk", tr.ID)
		return
	}
	slog.Info("SIP REGISTER 成功", "trunk", tr.ID, "expires", expire)
}

func (u *sipUA) optionsLoop(ctx context.Context) {
	t := time.NewTicker(25 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for i := range u.cfg.Trunks {
				u.sendOptions(ctx, u.cfg.Trunks[i])
			}
		}
	}
}

func (u *sipUA) sendOptions(ctx context.Context, tr config.SIPTrunkConfig) {
	if u.cli == nil {
		return
	}
	recipient := sip.Uri{Host: tr.Host, Port: tr.Port}
	req := sip.NewRequest(sip.OPTIONS, recipient)
	req.AppendHeader(sipAllowHeader())
	if strings.EqualFold(u.cfg.Transport, "tls") {
		req.SetTransport("TLS")
	} else {
		req.SetTransport("UDP")
	}
	tx, err := u.cli.TransactionRequest(ctx, req)
	if err != nil {
		slog.Warn("SIP OPTIONS 失败", "trunk", tr.ID, "err", err)
		return
	}
	defer tx.Terminate()
	_, _ = waitFinal(ctx, tx)
}

func waitFinal(ctx context.Context, tx sip.ClientTransaction) (*sip.Response, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tx.Done():
			return nil, fmt.Errorf("transaction done")
		case res := <-tx.Responses():
			if res.IsProvisional() {
				continue
			}
			return res, nil
		}
	}
}

func (u *sipUA) armSessionTimerFrom(sess *sipSession, raw, defaultRefresher string) {
	if sess == nil {
		return
	}
	sec, refresher := parseSessionExpires(raw)
	if sec < sipSessionMinSE {
		if u.cfg.SessionExpiresSec < sipSessionMinSE {
			return
		}
		sec = u.cfg.SessionExpiresSec
		refresher = defaultRefresher
	}
	if refresher == "" {
		refresher = defaultRefresher
	}
	weRefresh := (defaultRefresher == "uac" && refresher != "uas") || (defaultRefresher == "uas" && refresher == "uas")
	if !weRefresh {
		return
	}
	sess.mu.Lock()
	sess.sessionExp = time.Duration(sec) * time.Second
	if sess.refreshStop == nil {
		sess.refreshStop = make(chan struct{})
	}
	stop := sess.refreshStop
	interval := sess.sessionExp / 2
	sess.mu.Unlock()
	go u.sessionRefreshLoop(sess, stop, interval)
}

func (u *sipUA) sessionRefreshLoop(sess *sipSession, stop <-chan struct{}, interval time.Duration) {
	if interval < 30*time.Second {
		interval = 45 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			sess.mu.Lock()
			ended := sess.ended
			sess.mu.Unlock()
			if ended {
				return
			}
			u.sendSessionRefresh(sess)
		}
	}
}

func (u *sipUA) sendSessionRefresh(sess *sipSession) {
	sess.mu.Lock()
	cli, srv, se := sess.client, sess.server, sess.sessionExp
	sess.mu.Unlock()
	if se < time.Duration(sipSessionMinSE)*time.Second {
		return
	}
	dest, ok := refreshTarget(cli, srv)
	if !ok {
		return
	}
	req := sip.NewRequest(sip.UPDATE, dest)
	req.AppendHeader(sip.NewHeader("Supported", "timer"))
	req.AppendHeader(sip.NewHeader("Session-Expires", strconv.Itoa(int(se.Seconds()))+";refresher=uac"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var res *sip.Response
	var err error
	if cli != nil {
		res, err = cli.Do(ctx, req)
	} else if srv != nil {
		res, err = srv.Do(ctx, req)
	}
	if err != nil {
		slog.Warn("SIP session refresh 失败", "call_id", sess.callID, "err", err)
		return
	}
	if res != nil && res.StatusCode == sip.StatusMethodNotAllowed {
		u.sendReInviteRefresh(sess, dest)
	}
}

func (u *sipUA) sendReInviteRefresh(sess *sipSession, dest sip.Uri) {
	sess.mu.Lock()
	cli, srv, rtpSess := sess.client, sess.server, sess.rtp
	sess.mu.Unlock()
	if rtpSess == nil {
		return
	}
	sdp := buildAnswerSDP(u.cfg.AdvertiseHost(), rtpSess.localPort(), sdpMedia{Types: []int{int(rtpSess.currentPT())}})
	req := sip.NewRequest(sip.INVITE, dest)
	req.SetBody([]byte(sdp))
	req.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var err error
	if cli != nil {
		_, err = cli.Do(ctx, req)
	} else if srv != nil {
		_, err = srv.Do(ctx, req)
	}
	if err != nil {
		slog.Warn("SIP re-INVITE refresh 失败", "call_id", sess.callID, "err", err)
	}
}

func refreshTarget(cli *sipgo.DialogClientSession, srv *sipgo.DialogServerSession) (sip.Uri, bool) {
	if cli != nil && cli.InviteResponse != nil && cli.InviteResponse.Contact() != nil {
		return cli.InviteResponse.Contact().Address, true
	}
	if srv != nil && srv.InviteRequest != nil && srv.InviteRequest.Contact() != nil {
		return srv.InviteRequest.Contact().Address, true
	}
	return sip.Uri{}, false
}

func (s *sipSession) stopTimer() {
	s.refreshOnce.Do(func() {
		if s.refreshStop != nil {
			close(s.refreshStop)
		}
	})
}

func (u *sipUA) endCall(callID string, sendBye bool) {
	u.mu.Lock()
	sessions := make([]*sipSession, 0, len(u.byCall[callID]))
	for d := range u.byCall[callID] {
		sessions = append(sessions, d)
	}
	u.mu.Unlock()
	for _, d := range sessions {
		u.dropDialog(d)
		u.endDialog(d, sendBye)
	}
}

func (u *sipUA) endDialog(d *sipSession, sendBye bool) {
	d.stopTimer()
	d.mu.Lock()
	already := d.ended
	d.ended = true
	rtpSess := d.rtp
	cli := d.client
	srv := d.server
	confirmed, cancel := d.confirmed, d.cancel
	d.mu.Unlock()
	if !confirmed && cancel != nil {
		cancel()
	}
	if already {
		if rtpSess != nil {
			rtpSess.close()
		}
		return
	}
	if sendBye && confirmed {
		bctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if cli != nil {
			_ = cli.Bye(bctx)
		} else if srv != nil {
			_ = srv.Bye(bctx)
		}
		cancel()
	}
	if rtpSess != nil {
		rtpSess.close()
	}
}

func (u *sipUA) putDialog(d *sipSession) {
	u.mu.Lock()
	u.bySIP[d.sipCallID] = d
	if u.byCall[d.callID] == nil {
		u.byCall[d.callID] = map[*sipSession]struct{}{}
	}
	u.byCall[d.callID][d] = struct{}{}
	if d.dlgID != "" {
		u.byDlg[d.dlgID] = d
	}
	u.mu.Unlock()
}

func (u *sipUA) dropDialog(d *sipSession) {
	if d == nil {
		return
	}
	u.mu.Lock()
	delete(u.byCall[d.callID], d)
	if len(u.byCall[d.callID]) == 0 {
		delete(u.byCall, d.callID)
	}
	if u.bySIP[d.sipCallID] == d {
		delete(u.bySIP, d.sipCallID)
	}
	if d.dlgID != "" && u.byDlg[d.dlgID] == d {
		delete(u.byDlg, d.dlgID)
	}
	u.mu.Unlock()
}

func (u *sipUA) dialogBySIP(id string) *sipSession {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.bySIP[id]
}

func (u *sipUA) dialogByDlg(id string) *sipSession {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.byDlg[id]
}

func (u *sipUA) dialogByReq(req *sip.Request) *sipSession {
	if req == nil {
		return nil
	}
	if to := req.To(); to != nil {
		if _, ok := to.Params.Get("tag"); ok {
			if id, err := sip.DialogIDFromRequestUAS(req); err == nil {
				if d := u.dialogByDlg(id); d != nil {
					return d
				}
			}
			if id, err := sip.DialogIDFromRequestUAC(req); err == nil {
				if d := u.dialogByDlg(id); d != nil {
					return d
				}
			}
			return nil
		}
	}
	return u.dialogBySIP(headerCallID(req))
}

func headerCallID(msg sip.Message) string {
	if msg == nil {
		return ""
	}
	if h := msg.CallID(); h != nil {
		return h.Value()
	}
	return ""
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

func (u *sipUA) device(username, src string) *config.SIPDeviceConfig {
	ip := hostPortIP(src)
	for i := range u.cfg.Devices {
		d := &u.cfg.Devices[i]
		if d.Username != username {
			continue
		}
		for _, cidr := range d.AllowedCIDRs {
			n, err := config.ParseIPNet(cidr)
			if err == nil && n.Contains(ip) {
				return d
			}
		}
	}
	return nil
}

func (s *Service) SetDeviceHandler(h InboundSIPHandler) {
	if s.sip != nil {
		s.sip.onDevice = h
	}
}

func (s *Service) PrepareSIP(callID string) { s.enableSIPAudio(callID) }

func (u *sipUA) endLeg(callID, legID string) {
	u.mu.Lock()
	var found []*sipSession
	for d := range u.byCall[callID] {
		d.rtp.mu.Lock()
		match := d.rtp.legID == legID
		d.rtp.mu.Unlock()
		if match {
			found = append(found, d)
		}
	}
	u.mu.Unlock()
	for _, d := range found {
		u.dropDialog(d)
		u.endDialog(d, true)
	}
}
