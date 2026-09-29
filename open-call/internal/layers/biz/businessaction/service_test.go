package businessaction

import (
	"testing"

	"open-call/internal/integration/switchapi"
)

func TestPickOutcomeAndOutcomeKeys(t *testing.T) {
	out, err := pickOutcome([]string{"yes", "no"}, "yes")
	if err != nil || out != "yes" {
		t.Fatalf("pickOutcome: %q %v", out, err)
	}
	keys := outcomeKeys(map[string]any{"yes": "q", "no": "end"})
	if len(keys) != 2 {
		t.Fatalf("outcomeKeys: %v", keys)
	}
	_ = switchapi.Event{Type: "business_action.requested"}
}
