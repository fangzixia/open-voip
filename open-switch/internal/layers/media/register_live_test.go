package media

import (
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"uuid"
)

// 向运行中的 open-switch 发 REGISTER，验证本机 USB/软电话所用账号能否注册。
func TestLiveRegistrarUSBAccount(t *testing.T) {
	if os.Getenv("LIVE_SIP_PROBE") != "1" {
		t.Skip("set LIVE_SIP_PROBE=1")
	}
	user := envOr("SIP_PROBE_USER", "100163800")
	pass := envOr("SIP_PROBE_PASS", "tm559333")
	domain := envOr("SIP_PROBE_DOMAIN", "192.168.2.111")
	targets := []string{
		envOr("SIP_PROBE_TARGET", "127.0.0.1:5060"),
		"192.168.2.111:5060",
	}
	uris := []string{
		fmt.Sprintf("sip:%s", domain),
		fmt.Sprintf("sip:%s:5060", domain),
	}
	for _, target := range targets {
		for _, uri := range uris {
			t.Run(target+"_"+uri, func(t *testing.T) {
				if err := probeRegister(target, uri, user, pass, domain); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func probeRegister(serverAddr, requestURI, user, password, domain string) error {
	device, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}
	defer device.Close()
	remote, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return err
	}
	deviceAddr := device.LocalAddr().String()
	callID := uuid.New().String()
	register := func(cseq int, auth string) string {
		extra := ""
		if auth != "" {
			extra = "Authorization: " + auth + "\r\n"
		}
		return fmt.Sprintf("REGISTER %s SIP/2.0\r\nVia: SIP/2.0/UDP %s;branch=z9hG4bKprobe%d;rport\r\nFrom: <sip:%s@%s>;tag=probe\r\nTo: <sip:%s@%s>\r\nCall-ID: probe-%s\r\nCSeq: %d REGISTER\r\nContact: <sip:%s@%s>\r\nExpires: 300\r\nMax-Forwards: 70\r\n%sContent-Length: 0\r\n\r\n",
			requestURI, deviceAddr, cseq, user, domain, user, domain, callID, cseq, user, deviceAddr, extra)
	}
	read := func() ([]byte, error) {
		buf := make([]byte, 8192)
		device.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _, e := device.ReadFromUDP(buf)
		return buf[:n], e
	}
	var response []byte
	for cseq := 1; cseq <= 3; cseq++ {
		if _, err := device.WriteToUDP([]byte(register(cseq, "")), remote); err != nil {
			return err
		}
		response, err = read()
		if err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("%s %s: no response: %w", serverAddr, requestURI, err)
	}
	status := strings.SplitN(string(response), "\r\n", 1)[0]
	if !strings.HasPrefix(status, "SIP/2.0 401") {
		if strings.HasPrefix(status, "SIP/2.0 200") {
			return nil
		}
		return fmt.Errorf("%s %s: unexpected %q", serverAddr, requestURI, status)
	}
	auth, err := digestAuthorization(sipHeader(response, "WWW-Authenticate"), "REGISTER", requestURI, user, password, "")
	if err != nil {
		return err
	}
	if _, err := device.WriteToUDP([]byte(register(10, auth)), remote); err != nil {
		return err
	}
	response, err = read()
	if err != nil {
		return fmt.Errorf("%s %s: no response after auth: %w", serverAddr, requestURI, err)
	}
	status = strings.SplitN(string(response), "\r\n", 1)[0]
	if !strings.HasPrefix(status, "SIP/2.0 200") {
		return fmt.Errorf("%s %s: auth failed %q", serverAddr, requestURI, status)
	}
	return nil
}

// 向运行中的 open-switch 注册设备账号并呼叫 DID，验证 IVR 入呼链路（需队列已绑定 IVR、DID 指向队列）。
func TestLiveDeviceInviteDID(t *testing.T) {
	if os.Getenv("LIVE_SIP_PROBE") != "1" {
		t.Skip("set LIVE_SIP_PROBE=1")
	}
	user := envOr("SIP_PROBE_USER", "100163800")
	pass := envOr("SIP_PROBE_PASS", "tm859333")
	domain := envOr("SIP_PROBE_DOMAIN", "124.73.207.183")
	target := envOr("SIP_PROBE_TARGET", "127.0.0.1:5060")
	did := envOr("SIP_PROBE_DID", "8001")
	domainURI := fmt.Sprintf("sip:%s", domain)
	if err := probeRegister(target, domainURI, user, pass, domain); err != nil {
		t.Fatal("register:", err)
	}
	if err := probeInviteDID(target, domain, user, pass, did); err != nil {
		t.Fatal("invite:", err)
	}
}

func probeInviteDID(serverAddr, domain, user, password, did string) error {
	device, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}
	defer device.Close()
	remote, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return err
	}
	deviceAddr := device.LocalAddr().String()
	callID := uuid.New().String()
	fromTag := "probeinv"
	inviteURI := fmt.Sprintf("sip:%s@%s", did, domain)
	sdp := "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 4000 RTP/AVP 0 8 101\r\na=rtpmap:0 PCMU/8000\r\na=rtpmap:8 PCMA/8000\r\n"
	invite := func(cseq int, auth string) string {
		extra := ""
		if auth != "" {
			extra = "Authorization: " + auth + "\r\n"
		}
		return fmt.Sprintf("INVITE %s SIP/2.0\r\nVia: SIP/2.0/UDP %s;branch=z9hG4bKinv%d;rport\r\nFrom: <sip:%s@%s>;tag=%s\r\nTo: <sip:%s@%s>\r\nCall-ID: %s\r\nCSeq: %d INVITE\r\nContact: <sip:%s@%s>\r\nMax-Forwards: 70\r\nContent-Type: application/sdp\r\n%sContent-Length: %d\r\n\r\n%s",
			inviteURI, deviceAddr, cseq, user, domain, fromTag, did, domain, callID, cseq, user, deviceAddr, extra, len(sdp), sdp)
	}
	read := func() ([]byte, error) {
		buf := make([]byte, 16384)
		device.SetReadDeadline(time.Now().Add(8 * time.Second))
		n, _, e := device.ReadFromUDP(buf)
		return buf[:n], e
	}
	var response []byte
	for cseq := 1; cseq <= 2; cseq++ {
		if _, err := device.WriteToUDP([]byte(invite(cseq, "")), remote); err != nil {
			return err
		}
		response, err = read()
		if err != nil {
			return fmt.Errorf("no invite response: %w", err)
		}
		status := strings.SplitN(string(response), "\r\n", 1)[0]
		if strings.HasPrefix(status, "SIP/2.0 401") {
			auth, err := digestAuthorization(sipHeader(response, "WWW-Authenticate"), "INVITE", inviteURI, user, password, "")
			if err != nil {
				return err
			}
			if _, err := device.WriteToUDP([]byte(invite(10, auth)), remote); err != nil {
				return err
			}
			response, err = read()
			if err != nil {
				return err
			}
			status = strings.SplitN(string(response), "\r\n", 1)[0]
		}
		for strings.HasPrefix(status, "SIP/2.0 1") {
			response, err = read()
			if err != nil {
				return err
			}
			status = strings.SplitN(string(response), "\r\n", 1)[0]
		}
		if strings.HasPrefix(status, "SIP/2.0 200") {
			to := sipHeader(response, "To")
			ack := fmt.Sprintf("ACK %s SIP/2.0\r\nVia: SIP/2.0/UDP %s;branch=z9hG4bKack1;rport\r\nFrom: <sip:%s@%s>;tag=%s\r\nTo: %s\r\nCall-ID: %s\r\nCSeq: 10 ACK\r\nMax-Forwards: 70\r\nContent-Length: 0\r\n\r\n",
				inviteURI, deviceAddr, user, domain, fromTag, to, callID)
			if _, err := device.WriteToUDP([]byte(ack), remote); err != nil {
				return err
			}
			time.Sleep(6 * time.Second)
			return nil
		}
		if strings.HasPrefix(status, "SIP/2.0 4") || strings.HasPrefix(status, "SIP/2.0 5") || strings.HasPrefix(status, "SIP/2.0 6") {
			return fmt.Errorf("invite failed %q", status)
		}
	}
	return fmt.Errorf("unexpected invite flow %q", string(response))
}
