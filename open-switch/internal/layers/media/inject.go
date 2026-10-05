package media

import (
	"context"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-audio/wav"
	"github.com/google/uuid"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media"

	"open-switch/internal/scope"
)

// resolvePrompt 解析单租户素材目录下的 WAV 素材 ID。
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

func (s *Service) playWaitingTone(callID, targetLegID string, loop bool, seq uint64) {
	for {
		s.playToneToRoom(callID, targetLegID, time.Second, seq)
		if !loop {
			return
		}
		// 缺省等待音采用一秒回铃、三秒停顿，避免持续尖锐响声。
		if !s.waitPromptInterval(callID, 3*time.Second, seq) {
			return
		}
	}
}

func (s *Service) waitPromptInterval(callID string, duration time.Duration, seq uint64) bool {
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		room := s.getRoom(callID)
		if room == nil || room.promptSeq.Load() != seq {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

func (s *Service) playPCMToRoom(callID, targetLegID string, pcm []int16, rate int, loop bool, seq uint64) {
	if rate <= 0 {
		rate = 8000
	}
	step := rate / 8000
	if step < 1 {
		step = 1
	}
	frames := make([][]byte, 0, len(pcm)/(160*step)+1)
	for i := 0; i+160*step <= len(pcm); i += 160 * step {
		buf := make([]byte, 160)
		for j := 0; j < 160; j++ {
			buf[j] = linearToMulaw(pcm[i+j*step])
		}
		frames = append(frames, buf)
	}
	if len(frames) == 0 {
		s.playToneToRoom(callID, targetLegID, time.Second, seq)
		return
	}
	packet := rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 0, SSRC: rand.Uint32(), Timestamp: rand.Uint32()}}
	for {
		for _, payload := range frames {
			room := s.getRoom(callID)
			if room == nil || room.promptSeq.Load() != seq {
				return
			}
			room.mu.RLock()
			for legID, p := range room.peers {
				if targetLegID != "" && legID != targetLegID {
					continue
				}
				if p.audioSamp != nil {
					_ = p.audioSamp.WriteSample(media.Sample{Data: payload, Duration: 20 * time.Millisecond})
				}
			}
			packet.SequenceNumber++
			packet.Timestamp += 160
			packet.Payload = payload
			if raw, err := packet.Marshal(); err == nil {
				for rt := range room.sipRTP {
					if targetLegID != "" && rt.legID != targetLegID {
						continue
					}
					rt.writePCMU(raw)
				}
			}
			room.mu.RUnlock()
			time.Sleep(20 * time.Millisecond)
		}
		if !loop {
			return
		}
	}
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
		pcm = append(pcm, int16(buf.Data[i]))
	}
	if len(pcm) == 0 {
		return nil, 0, io.ErrUnexpectedEOF
	}
	return pcm, int(dec.SampleRate), nil
}
