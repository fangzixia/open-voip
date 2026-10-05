// 本文件负责SIP 注册服务器：REGISTER 鉴权、绑定维护与设备查找。
package media

import (
	"context"
	"log/slog"
	"net"
	"open-switch/internal/config"
	"open-switch/internal/ports"
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
)

// onRegister 验证话机凭据，维护注册绑定及过期时间。
func (u *sipUA) onRegister(req *sip.Request, tx sip.ServerTransaction) {
	logSIP("receive", req, "")
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
	u.persistBindings(aor, contacts)
}

// SetSIPBindingStore 注入注册绑定存储并恢复重启前仍有效的绑定（仅组合根调用）。
func (s *Service) SetSIPBindingStore(ctx context.Context, store ports.SIPBindingStore) {
	if s.sip == nil || store == nil {
		return
	}
	s.sip.bindings = store
	rows, err := store.LoadSIPBindings(ctx)
	if err != nil {
		slog.Warn("恢复 SIP 注册绑定失败", "err", err)
		return
	}
	restored := 0
	s.sip.mu.Lock()
	for _, row := range rows {
		b, ok := bindingFromRow(row)
		if !ok {
			continue
		}
		s.sip.binds[b.AOR] = append(s.sip.binds[b.AOR], b)
		restored++
	}
	s.sip.mu.Unlock()
	if restored > 0 {
		slog.Info("已恢复 SIP 注册绑定", "count", restored)
	}
}

func (u *sipUA) persistBindings(aor string, contacts []sipBinding) {
	if u.bindings == nil {
		return
	}
	rows := make([]ports.SIPBinding, 0, len(contacts))
	for _, b := range contacts {
		addr := ""
		if b.Addr != nil {
			addr = b.Addr.String()
		}
		rows = append(rows, ports.SIPBinding{AOR: b.AOR, ContactURI: b.Contact.String(), CallID: b.CallID, CSeq: b.CSeq, Addr: addr, ExpiresAt: b.ExpiresAt})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := u.bindings.ReplaceSIPBindings(ctx, aor, rows); err != nil {
		slog.Warn("持久化 SIP 注册绑定失败", "aor", aor, "err", err)
	}
}

func bindingFromRow(row ports.SIPBinding) (sipBinding, bool) {
	if row.AOR == "" || !row.ExpiresAt.After(time.Now()) {
		return sipBinding{}, false
	}
	var contact sip.Uri
	if err := sip.ParseUri(row.ContactURI, &contact); err != nil {
		return sipBinding{}, false
	}
	b := sipBinding{AOR: row.AOR, Contact: contact, CallID: row.CallID, CSeq: row.CSeq, ExpiresAt: row.ExpiresAt}
	if row.Addr != "" {
		if addr, err := net.ResolveUDPAddr("udp", row.Addr); err == nil {
			b.Addr = addr
		}
	}
	return b, true
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
			// 更新后的 Contact 应优先接收呼叫，否则旧注册记录可能
			// 持续收到所有 ACD 邀约。
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
