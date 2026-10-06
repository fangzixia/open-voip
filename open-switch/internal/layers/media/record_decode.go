package media

import (
	"sync"

	"github.com/pion/opus"
)

// hqRecordingRate SIP 场景主混音与分轨 WAV 的目标采样率（宽带存档）。
const hqRecordingRate = 16000

// decodeRTPAudio 将 RTP 音频载荷解码为线性 PCM 及采样率。
// PT=0/8 为 8k G.711；Opus 解码为 48k，后续由 addLinearPCM 重采样到录音率（通常 16k）。
func decodeRTPAudio(payloadType uint8, mime string, payload []byte) ([]int16, int) {
	if len(payload) == 0 {
		return nil, 0
	}
	switch payloadType {
	case 0:
		return pcmuPayloadToPCM(payload), 8000
	case 8:
		out := make([]int16, len(payload))
		for i, b := range payload {
			out[i] = alawToLinear(b)
		}
		return out, 8000
	default:
		if payloadType == opusPayloadType || mime == "audio/opus" {
			return decodeOpusMono(payload)
		}
		return nil, 0
	}
}

var (
	opusHQMu   sync.Mutex
	opusHQDec  opus.Decoder
	opusHQInit bool
)

// decodeOpusMono 将单帧 Opus 解码为 48 kHz 单声道 PCM（供录音混音）。
func decodeOpusMono(payload []byte) ([]int16, int) {
	opusHQMu.Lock()
	if !opusHQInit {
		opusHQDec, _ = opus.NewDecoderWithOutput(48000, 1)
		opusHQInit = true
	}
	dec := opusHQDec
	opusHQMu.Unlock()
	pcm := make([]int16, 5760)
	n, err := dec.DecodeToInt16(payload, pcm)
	if err != nil || n <= 0 {
		return nil, 0
	}
	return pcm[:n], 48000
}
