// 本文件负责SIP 会话定时器（RFC 4028）刷新。
package media

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

// armSessionTimerFrom 根据 Session-Expires 安排后续会话刷新。
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
	logSIP("send", req, sess.callID)
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
	logSIP("send", req, sess.callID)
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
