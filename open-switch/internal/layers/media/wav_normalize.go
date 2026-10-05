package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NormalizePromptUpload 将上传音频规范为 8 kHz 电话母带，并可选生成 48 kHz HD 副本。
func NormalizePromptUpload(ctx context.Context, ffmpegPath string, raw []byte) (narrow []byte, hd []byte, err error) {
	if len(raw) == 0 {
		return nil, nil, fmt.Errorf("empty wav")
	}
	bin := strings.TrimSpace(ffmpegPath)
	if bin != "" {
		narrow, err = ffmpegPromptWAV(ctx, bin, raw, 8000)
		if err != nil {
			return nil, nil, err
		}
		hd, _ = ffmpegPromptWAV(ctx, bin, raw, 48000)
		if len(hd) == 0 {
			hd = nil
		}
		return narrow, hd, nil
	}
	pcm, rate, err := decodeWAVBytes(raw)
	if err != nil {
		return nil, nil, err
	}
	pcm8, _ := downsamplePCMTo8k(pcm, rate)
	pcm8 = preparePromptPCM(pcm8)
	narrow = encodePCM16WAV(pcm8, 8000)
	pcm48 := resamplePCM(pcm, rate, 48000)
	if len(pcm48) > 0 {
		hd = encodePCM16WAV(pcm48, 48000)
	}
	return narrow, hd, nil
}

func ffmpegPromptWAV(ctx context.Context, bin string, in []byte, sampleRate int) ([]byte, error) {
	dir, err := os.MkdirTemp("", "open-switch-prompt-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	inPath := filepath.Join(dir, "in.wav")
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
		"-ac", "1", "-c:a", "pcm_s16le", "-f", "wav", outPath,
	}
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, trimTail(string(out), 400))
	}
	return os.ReadFile(outPath)
}

func trimTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func encodePCM16WAV(pcm []int16, rate int) []byte {
	if rate <= 0 {
		rate = 8000
	}
	dataLen := len(pcm) * 2
	buf := make([]byte, 44+dataLen)
	copy(buf, "RIFF")
	putLE32(buf[4:], uint32(36+dataLen))
	copy(buf[8:], "WAVEfmt ")
	putLE32(buf[16:], 16)
	putLE16(buf[20:], 1)
	putLE16(buf[22:], 1)
	putLE32(buf[24:], uint32(rate))
	putLE32(buf[28:], uint32(rate*2))
	putLE16(buf[32:], 2)
	putLE16(buf[34:], 16)
	copy(buf[36:], "data")
	putLE32(buf[40:], uint32(dataLen))
	for i, s := range pcm {
		putLE16(buf[44+i*2:], uint16(s))
	}
	return buf
}

func putLE16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func putLE32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func decodeWAVBytes(raw []byte) ([]int16, int, error) {
	dir, err := os.MkdirTemp("", "wav-decode-*")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(dir)
	p := filepath.Join(dir, "t.wav")
	if err := os.WriteFile(p, raw, 0600); err != nil {
		return nil, 0, err
	}
	return readPCMWav(p)
}
