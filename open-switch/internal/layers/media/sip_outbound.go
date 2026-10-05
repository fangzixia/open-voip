// 本文件负责SIP 外呼：INVITE 构造、PRACK 与中继选择。
package media

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"open-switch/internal/config"
	"open-switch/internal/errs"
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

// originate 选择中继或已注册话机，发起 SIP INVITE 并处理协商响应。
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
	logSDP("send", "offer", callID, sdp)
	if u.dlgCli == nil {
		rtpSess.close()
		return errs.Unprocessable("SIP 未就绪", errs.CodeSIPDisabled)
	}

	var peerIP net.IP
	if tr == nil {
		if addr := u.lookupReg(dial); addr != nil {
			peerIP = addr.IP
		}
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
		logSIP("send", dlg.InviteRequest, callID)
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
			peerIP:     peerIP,
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
				logSIP("receive", res, callID)
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
			sess.mu.Lock()
			sess.dlgID = dlg.ID
			sess.mu.Unlock()
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
	logSIP("send", req, "")
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
