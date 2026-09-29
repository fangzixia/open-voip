package media

import (
	"context"
	"github.com/google/uuid"
	"open-switch/internal/scope"
	"path/filepath"
	"testing"
)

// TestPromptApplicationIsolation 确认提示音路径按应用命名空间隔离。
func TestPromptApplicationIsolation(t *testing.T) {
	s := &Service{recDir: t.TempDir()}
	a := scope.WithApplication(context.Background(), "a")
	b := scope.WithApplication(context.Background(), "b")
	asset := uuid.NewString() + ".wav"
	if s.resolvePrompt(a, asset) == s.resolvePrompt(b, asset) {
		t.Fatal("shared prompt path")
	}
	for _, path := range []string{"../" + asset, filepath.Join(s.recDir, asset), "private.wav"} {
		if s.resolvePrompt(a, path) != "" {
			t.Fatalf("accepted unsafe prompt %q", path)
		}
	}
	if s.resolvePrompt(context.Background(), asset) != "" {
		t.Fatal("unscoped prompt access")
	}
}
