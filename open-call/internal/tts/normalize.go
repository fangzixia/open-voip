package tts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NormalizeToPromptWAV 使用 FFmpeg 将任意音频转为指定采样率的 mono PCM WAV。
func NormalizeToPromptWAV(ctx context.Context, ffmpegPath string, in []byte, sampleRate int) ([]byte, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("合成结果为空")
	}
	if sampleRate != 8000 && sampleRate != 16000 {
		return nil, fmt.Errorf("不支持的采样率 %d", sampleRate)
	}
	bin := strings.TrimSpace(ffmpegPath)
	if bin == "" {
		return nil, fmt.Errorf("FFmpeg 路径未配置")
	}
	dir, err := os.MkdirTemp("", "open-call-tts-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	inPath := filepath.Join(dir, "in.audio")
	outPath := filepath.Join(dir, "out.wav")
	if err := os.WriteFile(inPath, in, 0600); err != nil {
		return nil, err
	}
	af := "highpass=f=80,lowpass=f=3400,loudnorm=I=-16:TP=-2:LRA=7"
	if sampleRate > 8000 {
		af = "loudnorm=I=-16:TP=-2:LRA=7"
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", inPath,
		"-af", af,
		"-ar", fmt.Sprintf("%d", sampleRate),
		"-ac", "1",
		"-c:a", "pcm_s16le",
		"-f", "wav",
		outPath,
	}
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		msg := string(out)
		if len(msg) > 800 {
			msg = msg[len(msg)-800:]
		}
		return nil, fmt.Errorf("FFmpeg 转码失败: %w: %s", err, msg)
	}
	return os.ReadFile(outPath)
}
