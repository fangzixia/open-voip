package media

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// TestPromptPathValidation 确认仅接受 UUID.wav，并拒绝路径穿越。
func TestPromptPathValidation(t *testing.T) {
	s := &Service{audioRecDir: t.TempDir(), videoRecDir: t.TempDir()}
	ctx := context.Background()
	asset := uuid.New().String() + ".wav"
	got := s.resolvePrompt(ctx, asset)
	if got == "" {
		t.Fatal("expected resolved prompt path")
	}
	if filepath.Base(got) != asset {
		t.Fatalf("unexpected path %q", got)
	}
	for _, path := range []string{"../" + asset, filepath.Join(s.audioRecDir, asset), "private.wav"} {
		if s.resolvePrompt(ctx, path) != "" {
			t.Fatalf("accepted unsafe prompt %q", path)
		}
	}
}
