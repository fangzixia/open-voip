// 本文件负责SIP 中继注册与 OPTIONS 保活。
package media

import (
	"context"
	"fmt"
	"log/slog"
	"open-switch/internal/config"
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

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

// doRegister 向运营商中继发送 REGISTER，并处理挑战认证。
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
	logSIP("send", req, "")
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
	logSIP("receive", res, "")
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
		logSIP("send", newReq, "")
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
		logSIP("receive", res, "")
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
	logSIP("send", req, "")
	if err != nil {
		slog.Warn("SIP OPTIONS 失败", "trunk", tr.ID, "err", err)
		return
	}
	defer tx.Terminate()
	res, err := waitFinal(ctx, tx)
	if err != nil {
		slog.Debug("SIP OPTIONS 无响应", "trunk", tr.ID, "err", err)
		return
	}
	logSIP("receive", res, "")
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
