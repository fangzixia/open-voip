package aibot

import (
	"context"
	"testing"

	"open-call/internal/config"
)

func TestResolveSystemPromptFallback(t *testing.T) {
	cfg := config.AibotConfig{SystemPrompt: "global"}
	got := resolveSystemPrompt(context.Background(), cfg, QueuePromptStore{}, "q1")
	if got != "global" {
		t.Fatalf("expected global prompt, got %q", got)
	}
}
