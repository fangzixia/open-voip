package aibot

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"

	"open-call/internal/aibot/realtime"
)

// PlayWAVToPeer 解码 WAV 并以 8kHz PCMU 写入 WebRTC 发送轨。
//
// 非 8k 素材先 ResamplePCM 到 8k，再按 20ms/160 样本节拍写入，与 pcm_pacer 一致，
// 避免一次性塞满导致对端 jitter buffer 突发。用于外呼语音通知等预录素材。
func PlayWAVToPeer(ctx context.Context, peer *PeerSession, wavData []byte) (time.Duration, error) {
	pcm, rate, err := decodeWAV(wavData)
	if err != nil {
		return 0, err
	}
	if rate != 8000 {
		pcm = resamplePCM16(pcm, rate, 8000)
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

// PlaySilenceToPeer 按 20ms 帧发送 8kHz 静音，保持 RTP 直到挂断。
func PlaySilenceToPeer(ctx context.Context, peer *PeerSession, d time.Duration) error {
	if peer == nil || d <= 0 {
		return nil
	}
	frames := int(d / (20 * time.Millisecond))
	if frames < 1 {
		frames = 1
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	silence := make([]int16, 160)
	for i := 0; i < frames; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := peer.WritePCMU8k(silence); err != nil {
				return err
			}
		}
	}
	return nil
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

// resamplePCM16 播放前将 WAV 样本重采样到 8 kHz（与 realtime 算法一致）。
func resamplePCM16(in []int16, fromRate, toRate int) []int16 {
	return realtime.ResamplePCM(in, fromRate, toRate)
}
