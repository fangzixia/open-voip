package media

import (
	"sync"

	"github.com/pion/opus"
)

const hqRecordingRate = 16000

// decodeRTPAudio 将 RTP 音频载荷解码为线性 PCM 及采样率。
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
