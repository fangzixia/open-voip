package media

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
)

const sipAllowMethods = "INVITE, ACK, CANCEL, BYE, OPTIONS, REGISTER, PRACK, UPDATE"

func digestAuthorization(wwwAuth, method, uri, username, password, realm string) (string, error) {
	chal, err := digest.ParseChallenge(wwwAuth)
	if err != nil {
		return "", err
	}
	if realm != "" && chal.Realm == "" {
		chal.Realm = realm
	}
	chal.Algorithm = strings.ToUpper(chal.Algorithm)
	cred, err := digest.Digest(chal, digest.Options{
		Method:   method,
		URI:      uri,
		Username: username,
		Password: password,
		Count:    1,
	})
	if err != nil {
		return "", err
	}
	return cred.String(), nil
}

func headerHasToken(headers []sip.Header, token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return false
	}
	for _, h := range headers {
		if h == nil {
			continue
		}
		for _, p := range strings.Split(h.Value(), ",") {
			if strings.EqualFold(strings.TrimSpace(p), token) {
				return true
			}
		}
	}
	return false
}

func require100rel(headers []sip.Header) bool {
	return headerHasToken(headers, "100rel")
}

func newDigestNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

func registrarChallenge(realm, nonce string, stale bool) *digest.Challenge {
	return &digest.Challenge{
		Realm:     realm,
		Nonce:     nonce,
		Algorithm: "MD5",
		QOP:       []string{"auth"},
		Stale:     stale,
	}
}

func verifyRegistrarDigest(authHeader, method, aorUser, password string, nonceOK func(string) bool) bool {
	if password == "" || strings.TrimSpace(authHeader) == "" {
		return false
	}
	cred, err := digest.ParseCredentials(authHeader)
	if err != nil {
		return false
	}
	if !strings.EqualFold(cred.Username, aorUser) {
		return false
	}
	if nonceOK != nil && !nonceOK(cred.Nonce) {
		return false
	}
	qop := cred.QOP
	chal := &digest.Challenge{
		Realm:     cred.Realm,
		Nonce:     cred.Nonce,
		Algorithm: cred.Algorithm,
		Opaque:    cred.Opaque,
	}
	if qop != "" {
		chal.QOP = []string{qop}
	}
	opts := digest.Options{
		Method:   method,
		URI:      cred.URI,
		Username: cred.Username,
		Password: password,
		Count:    cred.Nc,
		Cnonce:   cred.Cnonce,
	}
	if opts.Count == 0 && qop != "" {
		opts.Count = 1
	}
	want, err := digest.Digest(chal, opts)
	if err != nil {
		return false
	}
	return cred.Response == want.Response
}

func parseSessionExpires(v string) (sec int, refresher string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, ""
	}
	parts := strings.Split(v, ";")
	sec, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		low := strings.ToLower(p)
		if strings.HasPrefix(low, "refresher=") {
			refresher = strings.ToLower(strings.TrimSpace(p[len("refresher="):]))
		}
	}
	return sec, refresher
}

func parseMinSE(v string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(v))
	return n
}

func parseRAck(v string) (rseq, cseq uint32, method string) {
	f := strings.Fields(strings.TrimSpace(v))
	if len(f) < 3 {
		return 0, 0, ""
	}
	rs, _ := strconv.ParseUint(f[0], 10, 32)
	cs, _ := strconv.ParseUint(f[1], 10, 32)
	return uint32(rs), uint32(cs), strings.ToUpper(f[2])
}

func redirectURI(res *sip.Response) (sip.Uri, bool) {
	if res == nil || res.StatusCode < 300 || res.StatusCode >= 400 {
		return sip.Uri{}, false
	}
	c := res.Contact()
	if c == nil || c.Address.Host == "" {
		return sip.Uri{}, false
	}
	cp := c.Address.Clone()
	if cp == nil {
		return sip.Uri{}, false
	}
	return *cp, true
}

func registerRequestURI(host string, port int) sip.Uri {
	return sip.Uri{Host: host, Port: port}
}

func sipAllowHeader() sip.Header {
	return sip.NewHeader("Allow", sipAllowMethods)
}
