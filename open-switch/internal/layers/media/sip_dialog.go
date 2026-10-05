// 本文件负责SIP 对话索引与结束处理。
package media

import (
	"context"
	"time"

	"github.com/emiago/sipgo/sip"
)

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

// inDialog 匹配对话内请求：必须带 To tag 并命中 dialog ID；只有尚未确定 dialog ID
// 的对话（外呼未应答）才允许退回按 Call-ID 匹配，避免仅凭 Call-ID 就能操纵通话。
func (u *sipUA) inDialog(req *sip.Request) *sipSession {
	if req == nil {
		return nil
	}
	if to := req.To(); to != nil {
		if _, ok := to.Params.Get("tag"); ok {
			if d := u.dialogByReq(req); d != nil {
				return d
			}
		}
	}
	d := u.dialogBySIP(headerCallID(req))
	if d == nil {
		return nil
	}
	d.mu.Lock()
	established := d.dlgID != ""
	d.mu.Unlock()
	if established {
		return nil
	}
	return d
}

// cancelMatches 要求 CANCEL 针对尚未应答的 INVITE：CSeq 序号一致且对话未确认。
func (u *sipUA) cancelMatches(d *sipSession, req *sip.Request) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.confirmed || d.server == nil {
		return false
	}
	cseq := req.CSeq()
	return cseq != nil && cseq.SeqNo == d.inviteCSeq
}

// trustedSource 校验对话内请求来源：建立对话的对端地址，或已配置中继的地址段。
func (u *sipUA) trustedSource(d *sipSession, src string) bool {
	ip := hostPortIP(src)
	if ip == nil {
		return false
	}
	d.mu.Lock()
	peer := d.peerIP
	d.mu.Unlock()
	if peer != nil && peer.Equal(ip) {
		return true
	}
	return u.ipAllowed(src)
}

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
