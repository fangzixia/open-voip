package media

import (
	"fmt"
	"net"
	"strings"
	"time"
)

type dnsCacheEntry struct {
	ips   []net.IP
	until time.Time
}

func (u *sipUA) lookupHost(host string) ([]net.IP, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, fmt.Errorf("empty host")
	}
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	now := time.Now()
	u.mu.Lock()
	if e, ok := u.dnsCache[host]; ok && now.Before(e.until) {
		ips := append([]net.IP(nil), e.ips...)
		u.mu.Unlock()
		return ips, nil
	}
	u.mu.Unlock()
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	u.mu.Lock()
	if u.dnsCache == nil {
		u.dnsCache = map[string]dnsCacheEntry{}
	}
	u.dnsCache[host] = dnsCacheEntry{ips: ips, until: now.Add(5 * time.Minute)}
	u.mu.Unlock()
	return ips, nil
}

func (u *sipUA) claimRTPPort(port int) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.rtpUsed == nil {
		u.rtpUsed = map[int]struct{}{}
	}
	if _, ok := u.rtpUsed[port]; ok {
		return false
	}
	u.rtpUsed[port] = struct{}{}
	return true
}

func (u *sipUA) releaseRTPPort(port int) {
	if port <= 0 {
		return
	}
	u.mu.Lock()
	delete(u.rtpUsed, port)
	u.mu.Unlock()
}
