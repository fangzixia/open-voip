package media

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media"
	"math/rand/v2"
)

// resolvePrompt 仅允许录音目录下的提示音文件，避免任意路径读取。
func (s *Service) resolvePrompt(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	root, err := filepath.Abs(s.recDir)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, "prompts", path)
	}
	clean, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(root, clean)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return clean
}

func (s *Service) playSourceToRoom(callID string, filePath string, loop bool) {
	if filePath != "" {
		if pcm, rate, err := readPCMWav(filePath); err == nil && len(pcm) > 0 {
			s.playPCMToRoom(callID, pcm, rate, loop)
			return
		}
	}
	dur := 2 * time.Second
	if loop {
		dur = 8 * time.Second
	}
	s.playToneToRoom(callID, dur)
}

func (s *Service) playPCMToRoom(callID string, pcm []int16, rate int, loop bool) {
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
		s.playToneToRoom(callID, 2*time.Second)
		return
	}
	rounds := 1
	if loop {
		rounds = 4
	}
	packet := rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 0, SSRC: rand.Uint32(), Timestamp: rand.Uint32()}}
	for r := 0; r < rounds; r++ {
		for _, payload := range frames {
			room := s.getRoom(callID)
			if room == nil {
				return
			}
			room.mu.RLock()
			for _, p := range room.peers {
				if p.audioSamp != nil {
					_ = p.audioSamp.WriteSample(media.Sample{Data: payload, Duration: 20 * time.Millisecond})
				}
			}
			packet.SequenceNumber++
			packet.Timestamp += 160
			packet.Payload = payload
			if raw, err := packet.Marshal(); err == nil {
				for rt := range room.sipRTP {
					rt.writePCMU(raw)
				}
			}
			room.mu.RUnlock()
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func readPCMWav(path string) ([]int16, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()
	hdr := make([]byte, 12)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return nil, 0, err
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" {
		return nil, 0, io.ErrUnexpectedEOF
	}
	var channels, bits, rate int
	var data []byte
	for {
		chunk := make([]byte, 8)
		if _, err := io.ReadFull(f, chunk); err != nil {
			break
		}
		size := int(binary.LittleEndian.Uint32(chunk[4:8]))
		if size > 32*1024*1024 {
			return nil, 0, io.ErrUnexpectedEOF
		}
		body := make([]byte, size)
		if _, err := io.ReadFull(f, body); err != nil {
			return nil, 0, err
		}
		if size%2 == 1 {
			_, _ = f.Read(make([]byte, 1))
		}
		id := string(chunk[0:4])
		switch id {
		case "fmt ":
			if len(body) < 16 || binary.LittleEndian.Uint16(body[0:2]) != 1 {
				return nil, 0, io.ErrUnexpectedEOF
			}
			channels = int(binary.LittleEndian.Uint16(body[2:4]))
			rate = int(binary.LittleEndian.Uint32(body[4:8]))
			bits = int(binary.LittleEndian.Uint16(body[14:16]))
		case "data":
			data = body
		}
	}
	if len(data) == 0 || bits != 16 || channels < 1 {
		return nil, 0, io.ErrUnexpectedEOF
	}
	n := len(data) / 2
	pcm := make([]int16, 0, n/channels)
	for i := 0; i+2 <= len(data); i += 2 * channels {
		pcm = append(pcm, int16(binary.LittleEndian.Uint16(data[i:i+2])))
	}
	return pcm, rate, nil
}
