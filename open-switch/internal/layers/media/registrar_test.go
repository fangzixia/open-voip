// 本文件验证registrar的关键行为。
package media

import (
	"github.com/emiago/sipgo/sip"
	"net"
	"open-switch/internal/config"
	"testing"
	"time"
)

func TestLatestDeviceRegistrationReceivesNextOffer(t *testing.T) {
	u := newSIPUA(config.SIPConfig{}, nil)
	first := sipBinding{AOR: "1002", Contact: sip.Uri{Scheme: "sip", User: "1002", Host: "127.0.0.1", Port: 5001}, Addr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5001}, ExpiresAt: time.Now().Add(time.Minute)}
	second := sipBinding{AOR: "1002", Contact: sip.Uri{Scheme: "sip", User: "1002", Host: "127.0.0.1", Port: 5002}, Addr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5002}, ExpiresAt: time.Now().Add(time.Minute)}
	u.upsertBindingLocked(first)
	u.upsertBindingLocked(second)
	if got := u.lookupReg("1002"); got == nil || got.Port != 5002 {
		t.Fatalf("newer contact was not selected: %v", got)
	}
	u.upsertBindingLocked(first)
	if got := u.lookupReg("1002"); got == nil || got.Port != 5001 {
		t.Fatalf("refreshed contact was not selected: %v", got)
	}
}

type responseCapture struct {
	sip.ServerTransaction
	response *sip.Response
}

func (r *responseCapture) Respond(v *sip.Response) error { r.response = v; return nil }

func TestDeviceDigestBindsAccountRealmURIAndNonce(t *testing.T) {
	const password = "device-1001-secret"
	u := newSIPUA(config.SIPConfig{LocalDomain: "call.test", Devices: []config.SIPDeviceConfig{{Username: "1001", Password: password, AllowedCIDRs: []string{"127.0.0.1/32"}}}}, nil)
	for _, tc := range []struct {
		name, user, method, uri, password string
		valid                             bool
	}{
		{"valid", "1001", "REGISTER", "sip:call.test", password, true},
		{"wrong account", "1002", "REGISTER", "sip:call.test", password, false},
		{"wrong password", "1001", "REGISTER", "sip:call.test", "not-the-password", false},
		{"wrong URI", "1001", "REGISTER", "sip:another.test", password, false},
		{"INVITE digest reuse", "1001", "INVITE", "sip:call.test", password, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u.nonces["nonce"] = time.Now().Add(time.Minute)
			auth, err := digestAuthorization(registrarChallenge("call.test", "nonce", false).String(), tc.method, tc.uri, tc.user, tc.password, "")
			if err != nil {
				t.Fatal(err)
			}
			req := sip.NewRequest(sip.REGISTER, sip.Uri{Scheme: "sip", Host: "call.test"})
			req.SetSource("127.0.0.1:5555")
			req.AppendHeader(sip.NewHeader("Authorization", auth))
			capture := &responseCapture{}
			if got := u.checkRegistrarAuth(req, capture, "1001"); got != tc.valid {
				t.Fatalf("got %v want %v", got, tc.valid)
			}
			if tc.valid && u.checkRegistrarAuth(req, capture, "1001") {
				t.Fatal("nonce replay accepted")
			}
		})
	}
	if u.device("1001", "192.0.2.1:5555") != nil {
		t.Fatal("device ACL bypass")
	}
}
