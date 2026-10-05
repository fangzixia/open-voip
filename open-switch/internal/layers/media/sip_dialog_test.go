package media

import (
	"net"
	"testing"

	"github.com/emiago/sipgo/sip"

	"open-switch/internal/config"
)

func inDialogRequest(method sip.RequestMethod, callID, fromTag, toTag string) *sip.Request {
	req := sip.NewRequest(method, sip.Uri{Scheme: "sip", User: "1001", Host: "switch.test"})
	cid := sip.CallIDHeader(callID)
	req.AppendHeader(&cid)
	from := &sip.FromHeader{Address: sip.Uri{Scheme: "sip", User: "caller", Host: "peer.test"}, Params: sip.NewParams()}
	from.Params.Add("tag", fromTag)
	req.AppendHeader(from)
	to := &sip.ToHeader{Address: sip.Uri{Scheme: "sip", User: "1001", Host: "switch.test"}, Params: sip.NewParams()}
	if toTag != "" {
		to.Params.Add("tag", toTag)
	}
	req.AppendHeader(to)
	req.AppendHeader(&sip.CSeqHeader{SeqNo: 1, MethodName: method})
	return req
}

func TestInDialogRequiresMatchingTagsOnceEstablished(t *testing.T) {
	u := newSIPUA(config.SIPConfig{}, nil)
	good := inDialogRequest(sip.BYE, "cid-1", "remote", "local")
	dlgID, err := sip.DialogIDFromRequestUAS(good)
	if err != nil {
		t.Fatal(err)
	}
	u.putDialog(&sipSession{callID: "call", sipCallID: "cid-1", dlgID: dlgID})

	if u.inDialog(good) == nil {
		t.Fatal("matching tags must find the dialog")
	}
	if u.inDialog(inDialogRequest(sip.BYE, "cid-1", "remote", "forged")) != nil {
		t.Fatal("wrong To tag must not fall back to Call-ID")
	}
	if u.inDialog(inDialogRequest(sip.BYE, "cid-1", "remote", "")) != nil {
		t.Fatal("missing To tag must not match an established dialog")
	}
}

func TestInDialogFallsBackToCallIDBeforeEstablished(t *testing.T) {
	u := newSIPUA(config.SIPConfig{}, nil)
	u.putDialog(&sipSession{callID: "call", sipCallID: "cid-2"})
	if u.inDialog(inDialogRequest(sip.BYE, "cid-2", "remote", "x")) == nil {
		t.Fatal("dialog without ID yet should match by Call-ID")
	}
}

func TestTrustedSource(t *testing.T) {
	u := newSIPUA(config.SIPConfig{}, nil)
	u.allowed = parseAllowedNets([]string{"10.0.0.0/24"})
	d := &sipSession{peerIP: net.ParseIP("192.0.2.10")}
	cases := map[string]bool{
		"192.0.2.10:5060":   true,
		"10.0.0.7:5060":     true,
		"198.51.100.1:5060": false,
	}
	for src, want := range cases {
		if got := u.trustedSource(d, src); got != want {
			t.Fatalf("%s: got %v want %v", src, got, want)
		}
	}
}

func TestCancelMatchesOnlyPendingInvite(t *testing.T) {
	u := newSIPUA(config.SIPConfig{}, nil)
	req := inDialogRequest(sip.CANCEL, "cid-3", "remote", "")
	d := &sipSession{inviteCSeq: 1}
	if u.cancelMatches(d, req) {
		t.Fatal("outbound (client) dialog must not accept inbound CANCEL")
	}
}
