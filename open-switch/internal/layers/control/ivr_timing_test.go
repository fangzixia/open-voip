package control

import (
	"testing"
	"time"
)

func TestPlayNodeWaitSecondsCeilsPromptDuration(t *testing.T) {
	if got := playNodeWaitSeconds(2, 3100*time.Millisecond); got != 4 {
		t.Fatalf("3.1s prompt with 2s config: got %d want 4", got)
	}
	if got := playNodeWaitSeconds(10, 800*time.Millisecond); got != 10 {
		t.Fatalf("configured larger: got %d", got)
	}
}
