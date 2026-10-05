package media

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"

	"open-switch/internal/config"
	"open-switch/internal/ports"
)

type memBindingStore struct {
	rows map[string][]ports.SIPBinding
}

func (m *memBindingStore) LoadSIPBindings(context.Context) ([]ports.SIPBinding, error) {
	var out []ports.SIPBinding
	for _, rows := range m.rows {
		out = append(out, rows...)
	}
	return out, nil
}

func (m *memBindingStore) ReplaceSIPBindings(_ context.Context, aor string, rows []ports.SIPBinding) error {
	m.rows[aor] = append([]ports.SIPBinding(nil), rows...)
	return nil
}

func TestSIPBindingsSurviveRestart(t *testing.T) {
	store := &memBindingStore{rows: map[string][]ports.SIPBinding{}}
	before := &Service{sip: newSIPUA(config.SIPConfig{}, nil)}
	before.sip.bindings = store
	b := sipBinding{
		AOR:       "1001",
		Contact:   sip.Uri{Scheme: "sip", User: "1001", Host: "10.0.0.8", Port: 5062},
		ExpiresAt: time.Now().Add(time.Hour),
		CallID:    "reg-1",
		CSeq:      7,
		Addr:      &net.UDPAddr{IP: net.ParseIP("203.0.113.5"), Port: 40000},
	}
	before.sip.persistBindings("1001", []sipBinding{b})

	after := &Service{sip: newSIPUA(config.SIPConfig{}, nil)}
	after.SetSIPBindingStore(context.Background(), store)
	addr := after.sip.lookupReg("1001")
	if addr == nil || addr.String() != "203.0.113.5:40000" {
		t.Fatalf("restored addr = %v", addr)
	}
	got := after.sip.binds["1001"][0]
	if !uriEqual(got.Contact, b.Contact) || got.CSeq != 7 || got.CallID != "reg-1" {
		t.Fatalf("restored binding = %+v", got)
	}
}

func TestExpiredBindingNotRestored(t *testing.T) {
	store := &memBindingStore{rows: map[string][]ports.SIPBinding{
		"1002": {{AOR: "1002", ContactURI: "sip:1002@10.0.0.9", Addr: "10.0.0.9:5060", ExpiresAt: time.Now().Add(-time.Minute)}},
	}}
	s := &Service{sip: newSIPUA(config.SIPConfig{}, nil)}
	s.SetSIPBindingStore(context.Background(), store)
	if s.sip.lookupReg("1002") != nil {
		t.Fatal("expired binding must not be restored")
	}
}
