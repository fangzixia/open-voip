// 本文件负责SIP 接入地址访问控制。
package media

import (
	"net"
	"strings"

	"open-switch/internal/config"
)

func parseAllowedNets(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs)+2)
	for _, c := range cidrs {
		n, err := config.ParseIPNet(c)
		if err == nil && n != nil {
			out = append(out, n)
		}
	}
	return out
}

func ipInNets(ip net.IP, nets []*net.IPNet) bool {
	if ip == nil || len(nets) == 0 {
		return false
	}
	for _, n := range nets {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

func hostPortIP(hostport string) net.IP {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	return net.ParseIP(host)
}

func ipNetFromIP(ip net.IP) *net.IPNet {
	if ip == nil {
		return nil
	}
	bits := 32
	if ip.To4() == nil {
		bits = 128
	} else {
		ip = ip.To4()
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
}
