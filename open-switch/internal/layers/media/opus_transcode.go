package media

import (
	"sync"

	"github.com/pion/opus"
)

const opusPayloadType = 111

var (
	opusDecMu sync.Mutex
	opusDec   opus.Decoder
)

func init() {
	opusDec, _ = opus.NewDecoderWithOutput(48000, 1)
}

// rtpPayloadToPCMU 将 RTP 音频载荷转为 20ms@8kHz 的 PCMU；无法识别时返回 nil。
func rtpPayloadToPCMU(payloadType uint8, payload []byte) []byte {
	switch payloadType {
	case 0:
		return payload
	case 8:
		return transcodeG711(8, 0, payload)
	case opusPayloadType:
		return opusToPCMU(payload)
	default:
		return nil
	}
}

// opusToPCMU 将 Opus 帧解码并重采样为 20 ms@8 kHz 的 PCMU 载荷。
func opusToPCMU(opusFrame []byte) []byte {
	if len(opusFrame) == 0 {
		return nil
	}
	pcm48 := make([]int16, 5760)
	opusDecMu.Lock()
	n, err := opusDec.DecodeToInt16(opusFrame, pcm48)
	opusDecMu.Unlock()
	if err != nil || n < 160 {
		return nil
	}
	pcm8 := resamplePCM(pcm48[:n], 48000, 8000)
	out := make([]byte, len(pcm8))
	for i, s := range pcm8 {
		out[i] = linearToMulaw(s)
	}
	return out
}

func pcmuPayloadToPCM(payload []byte) []int16 {
	out := make([]int16, len(payload))
	for i, b := range payload {
		out[i] = mulawToLinear(b)
	}
	return out
}
