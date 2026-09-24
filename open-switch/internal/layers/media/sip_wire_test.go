package media

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"net"
	"open-switch/internal/config"
	"open-switch/internal/ports/dto"
	"strings"
	"testing"
	"time"
)

// 使用模拟话机走真实 UDP 信令：REGISTER 挑战、鉴权注册、
// 服务端 INVITE、话机应答与 ACK；无需外部 PBX。
func TestSIPDeviceRegisterAndRingOverUDP(t *testing.T) {
	reservation := udpSocket(t)
	listen := reservation.LocalAddr().String()
	reservation.Close()
	device := udpSocket(t)
	deviceAddr := device.LocalAddr().String()
	cfg := config.SIPConfig{Enabled: true, Listen: listen, ExternalIP: "127.0.0.1", LocalDomain: "call.test", UserAgent: "open-switch", Transport: "udp", LocalRegistrar: true, RTPPortMin: 31000, RTPPortMax: 31100, Devices: []config.SIPDeviceConfig{{Username: "1001", Password: "device-1001-secret", AllowedCIDRs: []string{"127.0.0.1/32"}}}}
	svc, err := NewService(Options{SIP: cfg, RecordingsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- svc.ServeSIP(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("SIP server failed to stop")
		}
	})
	remote, err := net.ResolveUDPAddr("udp", listen)
	if err != nil {
		t.Fatal(err)
	}
	register := func(cseq int, authorization string) string {
		extra := ""
		if authorization != "" {
			extra = "Authorization: " + authorization + "\r\n"
		}
		return fmt.Sprintf("REGISTER sip:call.test SIP/2.0\r\nVia: SIP/2.0/UDP %s;branch=z9hG4bKreg%d;rport\r\nFrom: <sip:1001@call.test>;tag=phone\r\nTo: <sip:1001@call.test>\r\nCall-ID: register-test\r\nCSeq: %d REGISTER\r\nContact: <sip:1001@%s>\r\nExpires: 300\r\nMax-Forwards: 70\r\n%sContent-Length: 0\r\n\r\n", deviceAddr, cseq, cseq, deviceAddr, extra)
	}
	read := func() ([]byte, error) {
		buf := make([]byte, 8192)
		device.SetReadDeadline(time.Now().Add(time.Second))
		n, _, e := device.ReadFromUDP(buf)
		return buf[:n], e
	}
	var response []byte
	for i := 1; i <= 5; i++ {
		device.WriteToUDP([]byte(register(i, "")), remote)
		response, err = read()
		if err == nil {
			break
		}
	}
	if err != nil || !strings.HasPrefix(string(response), "SIP/2.0 401") {
		t.Fatalf("no registrar challenge: %s %v", response, err)
	}
	authorization, err := digestAuthorization(sipHeader(response, "WWW-Authenticate"), "REGISTER", "sip:call.test", "1001", "device-1001-secret", "")
	if err != nil {
		t.Fatal(err)
	}
	device.WriteToUDP([]byte(register(10, authorization)), remote)
	response, err = read()
	if err != nil || !strings.HasPrefix(string(response), "SIP/2.0 200") {
		t.Fatalf("registration failed: %s %v", response, err)
	}
	callID := uuid.NewString()
	svc.PrepareSIP(callID)
	if err := svc.CreateRoom(ctx, callID, dto.RoomOptions{SessionType: dto.SessionTypeAudio}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.sip.endCall(callID, false) })
	result := make(chan error, 1)
	go func() {
		dialCtx, stop := context.WithTimeout(ctx, 4*time.Second)
		defer stop()
		result <- svc.OriginateSIP(dialCtx, callID, "seat-leg", "1001", "@device")
	}()
	invite, err := read()
	if err != nil || !strings.HasPrefix(string(invite), "INVITE ") {
		t.Fatalf("no device INVITE: %s %v", invite, err)
	}
	mediaSocket := udpSocket(t)
	sdp := buildPCMUSDP("127.0.0.1", mediaSocket.LocalAddr().(*net.UDPAddr).Port)
	answer := fmt.Sprintf("SIP/2.0 200 OK\r\nVia: %s\r\nFrom: %s\r\nTo: %s;tag=phone-answer\r\nCall-ID: %s\r\nCSeq: %s\r\nContact: <sip:1001@%s>\r\nContent-Type: application/sdp\r\nContent-Length: %d\r\n\r\n%s", sipHeader(invite, "Via"), sipHeader(invite, "From"), sipHeader(invite, "To"), sipHeader(invite, "Call-ID"), sipHeader(invite, "CSeq"), deviceAddr, len(sdp), sdp)
	device.WriteToUDP([]byte(answer), remote)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("device answer did not complete")
	}
	ack, err := read()
	if err != nil || !strings.HasPrefix(string(ack), "ACK ") {
		t.Fatalf("missing ACK: %s %v", ack, err)
	}
}
