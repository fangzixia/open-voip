// 本文件验证sip的关键行为。
package media

import (
	"net"
	"strings"
	"testing"

	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"

	"open-switch/internal/config"
)

func TestExtractSIPUser(t *testing.T) {
	got := extractSIPUser(`"bob" <sip:bob@127.0.0.1>;tag=abc`)
	if got != "bob" {
		t.Fatalf("got %q", got)
	}
	if requestURIUser("INVITE sip:8001@127.0.0.1 SIP/2.0") != "8001" {
		t.Fatal("expected 8001")
	}
}

func TestParseAndBuildSDP(t *testing.T) {
	body := "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 192.168.2.111\r\nt=0 0\r\nm=audio 4000 RTP/AVP 0 8 101\r\n"
	m := parseSDP(body)
	if m.IP != "192.168.2.111" || m.Port != 4000 {
		t.Fatalf("got %+v", m)
	}
	if m.preferG711() != 0 {
		t.Fatalf("prefer PT %d", m.preferG711())
	}
	sdp := buildPCMUSDP("127.0.0.1", 10000)
	m2 := parseSDP(sdp)
	if m2.IP != "127.0.0.1" || m2.Port != 10000 {
		t.Fatalf("built %+v", m2)
	}
	if !strings.Contains(sdp, "PCMA/8000") || !strings.Contains(sdp, "PCMU/8000") {
		t.Fatalf("expected both codecs: %s", sdp)
	}
	pcmaOnly := "m=audio 4000 RTP/AVP 8 101\r\nc=IN IP4 1.1.1.1\r\n"
	if parseSDP(pcmaOnly).preferG711() != 8 {
		t.Fatal("expected PCMA")
	}
}

func TestViaRportAndToTag(t *testing.T) {
	via := "SIP/2.0/UDP 192.168.2.111:58308;rport;branch=z9hG4bKabc"
	addr := &net.UDPAddr{IP: net.ParseIP("192.168.2.111"), Port: 58308}
	got := viaWithRport(via, addr)
	if !containsAll(got, "rport=58308", "received=192.168.2.111") {
		t.Fatalf("via %s", got)
	}
	to := withToTag(`<sip:8001@127.0.0.1>`, "ov1")
	if to != `<sip:8001@127.0.0.1>;tag=ov1` {
		t.Fatalf("to %s", to)
	}
}

func TestSipHeaderAndContact(t *testing.T) {
	raw := []byte("INVITE sip:8001@127.0.0.1 SIP/2.0\r\nFrom: <sip:bob@127.0.0.1>;tag=1\r\nContact: <sip:bob@192.168.2.111:58308;ob>\r\n\r\nv=0\r\n")
	if sipHeader(raw, "From") == "" {
		t.Fatal("from")
	}
	if contactURI(sipHeader(raw, "Contact")) != "sip:bob@192.168.2.111:58308;ob" {
		t.Fatalf("contact %s", contactURI(sipHeader(raw, "Contact")))
	}
	got := sipURIAddr(`<sip:bob@192.168.2.111:58308;ob>`)
	if got == nil || got.Port != 58308 || got.IP.String() != "192.168.2.111" {
		t.Fatalf("addr %+v", got)
	}
	got = sipURIAddr("sip:bob@127.0.0.1")
	if got == nil || got.Port != 5060 || !got.IP.IsLoopback() {
		t.Fatalf("default port %+v", got)
	}
	if sipBody(raw) != "v=0\r\n" {
		t.Fatalf("body %q", sipBody(raw))
	}
}

func TestG711TranscodeRoundTrip(t *testing.T) {
	in := []byte{linearToMulaw(0), linearToMulaw(1234), linearToMulaw(-3000)}
	alaw := transcodeG711(0, 8, in)
	back := transcodeG711(8, 0, alaw)
	if len(back) != len(in) {
		t.Fatal("len")
	}
	same := transcodeG711(0, 0, in)
	if string(same) != string(in) {
		t.Fatal("identity")
	}
}

func TestIPACL(t *testing.T) {
	nets := parseAllowedNets([]string{"10.0.0.1/32", "192.168.1.0/24"})
	if !ipInNets(net.ParseIP("10.0.0.1"), nets) {
		t.Fatal("exact")
	}
	if !ipInNets(net.ParseIP("192.168.1.9"), nets) {
		t.Fatal("cidr")
	}
	if ipInNets(net.ParseIP("8.8.8.8"), nets) {
		t.Fatal("should deny")
	}
	if hostPortIP("1.2.3.4:5060").String() != "1.2.3.4" {
		t.Fatal("hostport")
	}
}

func TestDigestAuthorization(t *testing.T) {
	chal := `Digest realm="asterisk", nonce="abc123", algorithm=MD5, qop="auth"`
	got, err := digestAuthorization(chal, "REGISTER", "sip:u@host", "alice", "secret", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "Digest ") || !strings.Contains(got, "username=\"alice\"") {
		t.Fatalf("cred %s", got)
	}
}

func TestRequire100rel(t *testing.T) {
	h := sip.NewHeader("Require", "100rel, timer")
	if !require100rel([]sip.Header{h}) {
		t.Fatal("expected 100rel")
	}
	if require100rel([]sip.Header{sip.NewHeader("Require", "timer")}) {
		t.Fatal("timer only")
	}
}

func TestBuildAnswerSDPSubset(t *testing.T) {
	offer := parseSDP("m=audio 4000 RTP/AVP 0 8 101\r\nc=IN IP4 1.1.1.1\r\n")
	ans := buildAnswerSDP("127.0.0.1", 20000, offer)
	if !strings.Contains(ans, "m=audio 20000 RTP/AVP 0 101") {
		t.Fatalf("expected single PCMU + 101: %s", ans)
	}
	if strings.Contains(ans, "PCMA/8000") {
		t.Fatal("answer must not include unselected PCMA")
	}
	pcma := parseSDP("m=audio 4000 RTP/AVP 8\r\n")
	ans8 := buildAnswerSDP("127.0.0.1", 20001, pcma)
	if !strings.Contains(ans8, "RTP/AVP 8") || strings.Contains(ans8, "rtpmap:0") {
		t.Fatalf("pcma answer: %s", ans8)
	}
	if buildAnswerSDP("127.0.0.1", 1, parseSDP("m=audio 9 RTP/AVP 9\r\n")) != "" {
		t.Fatal("non-G.711 offer must yield empty answer")
	}
}

func TestSIPRTPSequenceStatistics(t *testing.T) {
	r := &sipRTP{}
	for _, seq := range []uint16{10, 11, 14, 13, 15} {
		r.observeInbound(seq, 160)
	}
	if r.rxPackets != 5 || r.rxBytes != 800 || r.sequenceGaps != 2 || r.outOfOrder != 1 {
		t.Fatalf("unexpected RTP stats: %+v", r)
	}
}

func TestRegisterRequestURIHasNoUser(t *testing.T) {
	uri := registerRequestURI("sip.carrier.example", 5060)
	if uri.User != "" {
		t.Fatalf("RFC 3261 REGISTER Request-URI must not contain userinfo, got %q", uri.User)
	}
	if uri.Host != "sip.carrier.example" || uri.Port != 5060 {
		t.Fatalf("got %+v", uri)
	}
}

func TestParseSessionExpiresAndRAck(t *testing.T) {
	sec, ref := parseSessionExpires("1800;refresher=uac")
	if sec != 1800 || ref != "uac" {
		t.Fatalf("%d %s", sec, ref)
	}
	if parseMinSE("90") != 90 {
		t.Fatal("min-se")
	}
	rseq, cseq, method := parseRAck("776656 1 INVITE")
	if rseq != 776656 || cseq != 1 || method != "INVITE" {
		t.Fatalf("%d %d %s", rseq, cseq, method)
	}
}

func TestRedirectURIFrom3xx(t *testing.T) {
	res := sip.NewResponse(sip.StatusMovedTemporarily, "Moved Temporarily")
	res.AppendHeader(&sip.ContactHeader{Address: sip.Uri{User: "bob", Host: "10.0.0.8", Port: 5060}})
	uri, ok := redirectURI(res)
	if !ok || uri.User != "bob" || uri.Host != "10.0.0.8" {
		t.Fatalf("got %+v ok=%v", uri, ok)
	}
	okRes := sip.NewResponse(200, "OK")
	if _, ok := redirectURI(okRes); ok {
		t.Fatal("2xx is not redirect")
	}
}

func TestVerifyRegistrarDigest(t *testing.T) {
	nonce := "abcnonce"
	chal := registrarChallenge("127.0.0.1", nonce, false)
	cred, err := digest.Digest(chal, digest.Options{
		Method:   "REGISTER",
		URI:      "sip:127.0.0.1",
		Username: "alice",
		Password: "changeme",
		Count:    1,
		Cnonce:   "cnonce",
	})
	if err != nil {
		t.Fatal(err)
	}
	ok := verifyRegistrarDigest(cred.String(), "REGISTER", "alice", "changeme", func(n string) bool { return n == nonce })
	if !ok {
		t.Fatal("expected match")
	}
	if verifyRegistrarDigest(cred.String(), "REGISTER", "bob", "changeme", func(n string) bool { return n == nonce }) {
		t.Fatal("username mismatch")
	}
	if verifyRegistrarDigest(cred.String(), "REGISTER", "alice", "changeme", func(string) bool { return false }) {
		t.Fatal("stale nonce")
	}
}

func TestInviteHeadersPAIOnlyOnTrunk(t *testing.T) {
	u := newSIPUA(config.SIPConfig{UserAgent: "open-voip", ExternalIP: "127.0.0.1", LocalDomain: "example.com"}, nil)
	ext := u.inviteHeaders("1001", nil, 0)
	for _, h := range ext {
		if strings.EqualFold(h.Name(), "P-Asserted-Identity") {
			t.Fatal("PAI must not be sent to untrusted UA")
		}
	}
	tr := &config.SIPTrunkConfig{FromUser: "0210000"}
	with := u.inviteHeaders("0210000", tr, 1800)
	foundPAI, foundSE, foundAllow := false, false, false
	for _, h := range with {
		switch strings.ToLower(h.Name()) {
		case "p-asserted-identity":
			foundPAI = true
		case "session-expires":
			foundSE = strings.Contains(h.Value(), "refresher=uac")
		case "allow":
			foundAllow = strings.Contains(h.Value(), "PRACK") && strings.Contains(h.Value(), "UPDATE")
		}
	}
	if !foundPAI || !foundSE || !foundAllow {
		t.Fatalf("trunk headers pai=%v se=%v allow=%v", foundPAI, foundSE, foundAllow)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
