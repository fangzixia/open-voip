package media

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/pion/sdp/v3"
)

func sipHeader(raw []byte, name string) string {
	prefix := strings.ToLower(name) + ":"
	for _, line := range strings.Split(string(raw), "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), prefix) {
			return strings.TrimSpace(line[len(name)+1:])
		}
	}
	return ""
}

func sipBody(raw []byte) string {
	s := string(raw)
	if i := strings.Index(s, "\r\n\r\n"); i >= 0 {
		return s[i+4:]
	}
	if i := strings.Index(s, "\n\n"); i >= 0 {
		return s[i+2:]
	}
	return ""
}

func extractSIPUser(h string) string {
	i := strings.Index(strings.ToLower(h), "sip:")
	if i < 0 {
		return strings.Trim(h, "<> ")
	}
	rest := h[i+4:]
	rest = strings.Trim(rest, "<> ")
	if j := strings.IndexAny(rest, "@>; \t"); j >= 0 {
		return rest[:j]
	}
	return rest
}

func requestURIUser(first string) string {
	parts := strings.Fields(first)
	if len(parts) < 2 {
		return ""
	}
	return extractSIPUser(parts[1])
}

func withToTag(to, tag string) string {
	if strings.Contains(strings.ToLower(to), ";tag=") || tag == "" {
		return to
	}
	return to + ";tag=" + tag
}

func viaWithRport(via string, raddr *net.UDPAddr) string {
	if via == "" || raddr == nil {
		return via
	}
	received := raddr.IP.String()
	rport := strconv.Itoa(raddr.Port)
	lower := strings.ToLower(via)
	if !strings.Contains(lower, "rport=") {
		if i := strings.Index(lower, "rport"); i >= 0 {
			via = via[:i] + "rport=" + rport + via[i+5:]
			lower = strings.ToLower(via)
		}
	}
	if !strings.Contains(lower, "received=") {
		via += ";received=" + received
	}
	return via
}

type sdpMedia struct {
	IP     string
	Port   int
	Types  []int
	rtpmap map[int]string
	opusPT int // 动态 PT；-1 表示 offer 未含 Opus
}

func parseSDP(body string) sdpMedia {
	out := sdpMedia{opusPT: -1, rtpmap: map[int]string{}}
	var session sdp.SessionDescription
	if session.UnmarshalString(body) != nil {
		return out
	}
	var audio *sdp.MediaDescription
	for _, media := range session.MediaDescriptions {
		if media.MediaName.Media == "audio" {
			// One SIP leg has one audio input and one output producer.
			if audio != nil {
				return out
			}
			audio = media
		}
	}
	if audio == nil || strings.Join(audio.MediaName.Protos, "/") != "RTP/AVP" {
		return out
	}
	out.Port = audio.MediaName.Port.Value
	connection := audio.ConnectionInformation
	if connection == nil {
		connection = session.ConnectionInformation
	}
	if connection != nil && connection.Address != nil && connection.NetworkType == "IN" && connection.AddressType == "IP4" {
		out.IP = connection.Address.Address
	}
	for _, format := range audio.MediaName.Formats {
		if pt, err := strconv.Atoi(format); err == nil {
			out.Types = append(out.Types, pt)
		}
	}
	// Attribute scope belongs to Pion. A video c= or rtpmap can never replace
	// the selected audio address, codec or clock.
	for _, attr := range audio.Attributes {
		if attr.Key == "rtpmap" {
			f := strings.Fields(attr.Value)
			if len(f) < 2 {
				continue
			}
			pt, err := strconv.Atoi(f[0])
			if err != nil {
				continue
			}
			mapping := strings.ToUpper(f[1])
			if previous, ok := out.rtpmap[pt]; ok && previous != mapping {
				mapping = "INVALID"
			}
			out.rtpmap[pt] = mapping
			if strings.HasPrefix(strings.ToLower(f[1]), "opus/") {
				out.opusPT = pt
			}
		}
	}
	return out
}

func (m sdpMedia) hasPT(pt int) bool {
	for _, t := range m.Types {
		if t == pt {
			return true
		}
	}
	return false
}

func (m sdpMedia) hasAudioCodec() bool {
	if len(m.Types) == 0 {
		return false
	}
	for _, t := range m.Types {
		if m.validG711PT(t) {
			return true
		}
	}
	return false
}

func (m sdpMedia) validG711PT(pt int) bool {
	name := ""
	switch pt {
	case 0:
		name = "PCMU"
	case 8:
		name = "PCMA"
	default:
		return false
	}
	mapping, explicit := m.rtpmap[pt]
	return !explicit || mapping == name+"/8000" || mapping == name+"/8000/1"
}

func (m sdpMedia) negotiateCodec() (sipAudioCodec, uint8) {
	pt := m.preferG711()
	return codecFromPT(pt), pt
}

func (m sdpMedia) preferG711() uint8 {
	for _, t := range m.Types {
		if !m.validG711PT(t) {
			continue
		}
		if t == 8 {
			return 8
		}
		if t == 0 {
			return 0
		}
	}
	if len(m.Types) == 0 {
		return 0
	}
	return 0
}

func buildAudioSDP(ip string, port int, codecs []string) string {
	offerU, offerA := true, true
	if len(codecs) > 0 {
		offerU, offerA = false, false
		for _, c := range codecs {
			switch strings.ToUpper(strings.TrimSpace(c)) {
			case "PCMU":
				offerU = true
			case "PCMA":
				offerA = true
			}
		}
	}
	if !offerU && !offerA {
		return ""
	}
	pts := make([]string, 0, 6)
	var b strings.Builder
	if offerU {
		pts = append(pts, "0")
		b.WriteString("a=rtpmap:0 PCMU/8000\r\n")
	}
	if offerA {
		pts = append(pts, "8")
		b.WriteString("a=rtpmap:8 PCMA/8000\r\n")
	}
	pts = append(pts, "101")
	b.WriteString("a=rtpmap:101 telephone-event/8000\r\n")
	b.WriteString("a=fmtp:101 0-16\r\n")
	b.WriteString("a=ptime:20\r\n")
	b.WriteString("a=sendrecv\r\n")
	return fmt.Sprintf("v=0\r\n"+
		"o=open-voip 1 1 IN IP4 %s\r\n"+
		"s=call\r\n"+
		"c=IN IP4 %s\r\n"+
		"t=0 0\r\n"+
		"m=audio %d RTP/AVP %s\r\n%s", ip, ip, port, strings.Join(pts, " "), b.String())
}

func buildPCMUSDP(ip string, port int) string {
	return buildAudioSDP(ip, port, []string{"PCMU", "PCMA"})
}

func (u *sipUA) buildAnswerForCall(callID string, rtpPort int, offer sdpMedia) string {
	return buildAnswerSDP(u.cfg.AdvertiseHost(), rtpPort, offer)
}

// buildAnswerSDP 按 RFC 3264 对 offer 取子集：应答单一音频 PT，telephone-event 仅在 offer 中出现时带回。
func buildAnswerSDP(ip string, port int, offer sdpMedia) string {
	if !offer.hasAudioCodec() {
		return ""
	}
	codec, pt := offer.negotiateCodec()
	var b strings.Builder
	pts := []string{strconv.Itoa(int(pt))}
	switch codec {
	case sipCodecPCMA:
		b.WriteString("a=rtpmap:8 PCMA/8000\r\n")
	default:
		b.WriteString("a=rtpmap:0 PCMU/8000\r\n")
	}
	if offer.hasPT(101) {
		pts = append(pts, "101")
		b.WriteString("a=rtpmap:101 telephone-event/8000\r\n")
		b.WriteString("a=fmtp:101 0-16\r\n")
	}
	b.WriteString("a=ptime:20\r\n")
	b.WriteString("a=sendrecv\r\n")
	return fmt.Sprintf("v=0\r\n"+
		"o=open-voip 1 1 IN IP4 %s\r\n"+
		"s=call\r\n"+
		"c=IN IP4 %s\r\n"+
		"t=0 0\r\n"+
		"m=audio %d RTP/AVP %s\r\n%s", ip, ip, port, strings.Join(pts, " "), b.String())
}

func contactURI(h string) string {
	h = strings.TrimSpace(h)
	if i := strings.Index(h, "<"); i >= 0 {
		if j := strings.Index(h[i:], ">"); j > 0 {
			return h[i+1 : i+j]
		}
	}
	if i := strings.Index(strings.ToLower(h), "sip:"); i >= 0 {
		rest := h[i:]
		if j := strings.IndexAny(rest, " ;>"); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	return h
}

// sipURIAddr 从 Contact / Request-URI 解析 UDP 地址；无端口时默认 5060。
func sipURIAddr(uri string) *net.UDPAddr {
	u := strings.TrimSpace(uri)
	u = strings.TrimPrefix(u, "<")
	u = strings.TrimSuffix(u, ">")
	if i := strings.Index(strings.ToLower(u), "sip:"); i >= 0 {
		u = u[i+4:]
	} else if i := strings.Index(strings.ToLower(u), "sips:"); i >= 0 {
		u = u[i+5:]
	}
	if i := strings.Index(u, "@"); i >= 0 {
		u = u[i+1:]
	}
	if i := strings.IndexAny(u, ";>"); i >= 0 {
		u = u[:i]
	}
	u = strings.TrimSpace(u)
	if u == "" {
		return nil
	}
	host, port := u, 5060
	if h, p, err := net.SplitHostPort(u); err == nil {
		host = h
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			port = n
		}
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip == nil {
		addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			return nil
		}
		return addr
	}
	return &net.UDPAddr{IP: ip, Port: port}
}
