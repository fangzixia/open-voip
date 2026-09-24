package media

import (
	"context"
	"github.com/pion/rtp"
	"net"
	"open-switch/internal/config"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"testing"
	"time"
)

func udpSocket(t *testing.T) *net.UDPConn {
	t.Helper()
	c, e := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestTwoSIPLegsBridgeAndRejectForeignRTP(t *testing.T) {
	a, b := udpSocket(t), udpSocket(t)
	peerA, peerB := udpSocket(t), udpSocket(t)
	ra := &sipRTP{conn: a, remote: peerA.LocalAddr().(*net.UDPAddr), legID: "customer"}
	rb := &sipRTP{conn: b, remote: peerB.LocalAddr().(*net.UDPAddr), legID: "agent"}
	s := &Service{rooms: map[string]*room{"c": {peers: map[string]*peer{}, sipRTP: map[*sipRTP]struct{}{ra: {}, rb: {}}, dtmf: map[string]ports.DTMFHandler{}}}}
	go s.sipReadLoop("c", ra)
	go s.sipReadLoop("c", rb)
	raw, _ := (&rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 0, SequenceNumber: 7, Timestamp: 160, SSRC: 42}, Payload: []byte{1, 2, 3}}).Marshal()
	if _, err := peerA.WriteToUDP(raw, a.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	peerB.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1500)
	n, _, err := peerB.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	var packet rtp.Packet
	if err := packet.Unmarshal(buf[:n]); err != nil || string(packet.Payload) != "\x01\x02\x03" {
		t.Fatalf("bridge failed: %v %+v", err, packet)
	}
	if ra.acceptSource(&net.UDPAddr{IP: net.IPv4(192, 0, 2, 9), Port: 4444}) {
		t.Fatal("foreign source hijacked RTP")
	}
	if err := s.SetHold(context.Background(), "c", "agent", true); err != nil {
		t.Fatal(err)
	}
	if !rb.blocked() {
		t.Fatal("SIP hold did not affect media")
	}
}

func TestMultipleSIPDialogsAndUnknownTrunk(t *testing.T) {
	u := newSIPUA(config.SIPConfig{Trunks: []config.SIPTrunkConfig{{ID: "configured"}}}, nil)
	a, b := &sipSession{callID: "call", sipCallID: "a", dlgID: "da"}, &sipSession{callID: "call", sipCallID: "b", dlgID: "db"}
	u.putDialog(a)
	u.putDialog(b)
	u.dropDialog(a)
	if len(u.byCall["call"]) != 1 || u.dialogBySIP("b") != b {
		t.Fatal("dropping one SIP leg lost its peer")
	}
	if u.pickTrunk("unknown") != nil {
		t.Fatal("unknown trunk fell back to default")
	}
	u.endCall("call", false)
	if len(u.byCall) != 0 || len(u.bySIP) != 0 || len(u.byDlg) != 0 {
		t.Fatal("dialog index leak")
	}
}

func TestRepeatedIdenticalDTMFDigits(t *testing.T) {
	socket, peer := udpSocket(t), udpSocket(t)
	received := make(chan string, 2)
	s := &Service{rooms: map[string]*room{"c": {dtmf: map[string]ports.DTMFHandler{"customer": func(_ context.Context, d dto.DTMFDigit) { received <- string(d) }}}}}
	sess := &sipRTP{conn: socket, remote: peer.LocalAddr().(*net.UDPAddr)}
	go s.sipReadLoop("c", sess)
	for _, ts := range []uint32{160, 160, 320} {
		raw, _ := (&rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 101, Timestamp: ts}, Payload: []byte{1, 0x80, 0, 160}}).Marshal()
		peer.WriteToUDP(raw, socket.LocalAddr().(*net.UDPAddr))
	}
	for i := 0; i < 2; i++ {
		select {
		case digit := <-received:
			if digit != "1" {
				t.Fatal(digit)
			}
		case <-time.After(time.Second):
			t.Fatal("repeated digit lost")
		}
	}
}
