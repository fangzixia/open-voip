package media

import (
	"context"
	"io"

	"os"
	"path/filepath"
	"strings"

	"github.com/go-audio/wav"
	"github.com/google/uuid"

	"open-switch/internal/scope"
)

func (s *Service) resolvePrompt(ctx context.Context, path string) string {
	_ = ctx
	if filepath.Ext(path) != ".wav" {
		return ""
	}
	if _, err := uuid.Parse(strings.TrimSuffix(path, ".wav")); err != nil {
		return ""
	}
	return filepath.Join(s.audioRecDir, "prompts", scope.AssetNamespace(), path)
}

func readPCMWav(path string) ([]int16, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()
	dec := wav.NewDecoder(f)
	if !dec.IsValidFile() {
		return nil, 0, io.ErrUnexpectedEOF
	}
	buf, err := dec.FullPCMBuffer()
	if err != nil {
		return nil, 0, err
	}
	if dec.BitDepth != 16 || dec.NumChans < 1 {
		return nil, 0, io.ErrUnexpectedEOF
	}
	ch := int(dec.NumChans)
	pcm := make([]int16, 0, len(buf.Data)/ch)
	for i := 0; i < len(buf.Data); i += ch {
		var sum int64
		for j := 0; j < ch && i+j < len(buf.Data); j++ {
			sum += int64(buf.Data[i+j])
		}
		pcm = append(pcm, int16(sum/int64(ch)))
	}
	if len(pcm) == 0 {
		return nil, 0, io.ErrUnexpectedEOF
	}
	return pcm, int(dec.SampleRate), nil
}
