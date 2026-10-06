package aibot

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

// PlayWAVToPeer 解码 WAV 并以 8kHz PCMU 写入 WebRTC 发送轨。
func PlayWAVToPeer(ctx context.Context, peer *PeerSession, wavData []byte) (time.Duration, error) {
	pcm, rate, err := decodeWAV(wavData)
	if err != nil {
		return 0, err
	}
	if rate != 8000 {
		pcm = resamplePCM(pcm, rate, 8000)
	}
	if len(pcm) == 0 {
		return 0, nil
	}
	dur := time.Duration(len(pcm)) * time.Second / 8000
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	off := 0
	const frame = 160
	for off < len(pcm) {
		select {
		case <-ctx.Done():
			return dur, ctx.Err()
		case <-ticker.C:
			end := off + frame
			if end > len(pcm) {
				end = len(pcm)
			}
			chunk := pcm[off:end]
			if len(chunk) < frame {
				pad := make([]int16, frame)
				copy(pad, chunk)
				chunk = pad
			}
			if err := peer.WritePCMU8k(chunk); err != nil {
				return dur, err
			}
			off += frame
		}
	}
	return dur, nil
}

func decodeWAV(data []byte) ([]int16, int, error) {
	dec := wav.NewDecoder(bytes.NewReader(data))
	if !dec.IsValidFile() {
		return nil, 0, io.ErrUnexpectedEOF
	}
	buf, err := dec.FullPCMBuffer()
	if err != nil {
		return nil, 0, err
	}
	rate := int(dec.SampleRate)
	if rate <= 0 {
		rate = 8000
	}
	return intBufferToMono16(buf), rate, nil
}

func intBufferToMono16(buf *audio.IntBuffer) []int16 {
	if buf == nil || len(buf.Data) == 0 {
		return nil
	}
	ch := buf.Format.NumChannels
	if ch <= 1 {
		out := make([]int16, len(buf.Data))
		for i, v := range buf.Data {
			out[i] = int16(v)
		}
		return out
	}
	frames := len(buf.Data) / ch
	out := make([]int16, frames)
	for i := 0; i < frames; i++ {
		var sum int
		for c := 0; c < ch; c++ {
			sum += buf.Data[i*ch+c]
		}
		out[i] = int16(sum / ch)
	}
	return out
}

func resamplePCM(in []int16, fromRate, toRate int) []int16 {
	if fromRate <= 0 || toRate <= 0 || fromRate == toRate {
		return in
	}
	outLen := len(in) * toRate / fromRate
	if outLen == 0 {
		return nil
	}
	out := make([]int16, outLen)
	for i := range out {
		src := i * fromRate / toRate
		if src >= len(in) {
			src = len(in) - 1
		}
		out[i] = in[src]
	}
	return out
}
