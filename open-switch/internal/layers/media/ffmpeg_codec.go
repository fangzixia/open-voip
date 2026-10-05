package media

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// pcm16kToG722Frames 使用 FFmpeg 将 16 kHz PCM 切成 20ms G.722 RTP 帧（160 字节）。
func pcm16kToG722Frames(ctx context.Context, ffmpegPath string, pcm []int16) ([][]byte, error) {
	bin := strings.TrimSpace(ffmpegPath)
	if bin == "" {
		return nil, fmt.Errorf("ffmpeg not configured")
	}
	dir, err := os.MkdirTemp("", "g722-enc-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	wav := encodePCM16WAV(pcm, 16000)
	inPath := filepath.Join(dir, "in.wav")
	outPath := filepath.Join(dir, "out.g722")
	if err := os.WriteFile(inPath, wav, 0600); err != nil {
		return nil, err
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", inPath,
		"-c:a", "g722",
		"-f", "g722",
		outPath,
	}
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("g722 encode: %w: %s", err, trimTail(string(out), 300))
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		return nil, err
	}
	const frameLen = 160
	frames := make([][]byte, 0, len(raw)/frameLen+1)
	for i := 0; i < len(raw); i += frameLen {
		end := i + frameLen
		if end > len(raw) {
			end = len(raw)
		}
		f := make([]byte, frameLen)
		copy(f, raw[i:end])
		frames = append(frames, f)
	}
	return frames, nil
}

type ffmpegG722Decoder struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	in  io.WriteCloser
	out io.ReadCloser
}

func newFFmpegG722Decoder(ffmpegPath string) (*ffmpegG722Decoder, error) {
	bin := strings.TrimSpace(ffmpegPath)
	if bin == "" {
		return nil, fmt.Errorf("ffmpeg not configured")
	}
	cmd := exec.Command(bin,
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "g722", "-i", "pipe:0",
		"-f", "s16le", "-ar", "16000", "-ac", "1",
		"pipe:1",
	)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &ffmpegG722Decoder{cmd: cmd, in: in, out: out}, nil
}

func (d *ffmpegG722Decoder) decodeFrame(g722 []byte) ([]int16, error) {
	if d == nil || len(g722) == 0 {
		return nil, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.in.Write(g722); err != nil {
		return nil, err
	}
	buf := make([]byte, 320*2)
	n, err := io.ReadFull(d.out, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	samples := n / 2
	pcm := make([]int16, samples)
	for i := 0; i < samples; i++ {
		pcm[i] = int16(int(buf[i*2]) | int(buf[i*2+1])<<8)
	}
	return pcm, nil
}

func (d *ffmpegG722Decoder) close() {
	if d == nil {
		return
	}
	_ = d.in.Close()
	_ = d.cmd.Wait()
}
