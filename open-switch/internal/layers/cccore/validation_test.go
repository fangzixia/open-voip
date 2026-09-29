package cccore

import (
	"github.com/google/uuid"
	"open-switch/internal/ports"
	"testing"
)

func TestPublishedGraphAndCalendarValidation(t *testing.T) {
	for _, raw := range []string{`always`, `{"timezone":"Asia/Hong_Kong","mon":"09:00-18:00","2026-10-01":"closed"}`} {
		if err := validateHours(raw); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	for _, raw := range []string{`{"mon":"25:00-26:00"}`, `{"timezone":"invalid"}`, `{"mon":"18:00-09:00"}`, `{"monday":"09:00-18:00"}`, `{"2026-02-30":"closed"}`} {
		if err := validateHours(raw); err == nil {
			t.Fatalf("accepted calendar %s", raw)
		}
	}
	for _, raw := range []string{
		`{"start":"x","nodes":{"x":{"type":"business_action","action":"customer.check","choices":{"yes":"end"},"timeout_sec":5},"end":{"type":"hangup"}}}`,
		`{"start":"x","nodes":{"x":{"type":"time_check","open":"end","closed":"end"},"end":{"type":"hangup"}}}`,
		`{"start":"x","nodes":{"x":{"type":"play","next":"x"}}}`,
		`{"start":"x","nodes":{"x":{"type":"tts"}}}`,
	} {
		if err := validateIVR(raw, map[string]bool{}); err == nil {
			t.Fatalf("accepted invalid graph %s", raw)
		}
	}
}

func TestDuplicateExtensionsRejected(t *testing.T) {
	b := ports.ConfigBundle{Agents: []ports.AgentConfig{{ID: uuid.NewString(), UserRef: "one", Extension: "1001"}, {ID: uuid.NewString(), UserRef: "two", Extension: "1001"}}}
	normalizeBundle(&b)
	if err := validateBundle(b); err == nil {
		t.Fatal("duplicate extension accepted")
	}
}
