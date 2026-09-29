package ffmpeg

import (
	"os/exec"
	"testing"
)

func TestResolveRequiresFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("PATH 中无 ffmpeg，跳过")
	}
	path, err := Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("expected path")
	}
}
