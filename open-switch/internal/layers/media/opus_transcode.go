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

func opusToPCMU(opusFrame []byte) []byte {
	if len(opusFrame) == 0 {
		return nil
	}
	pcm48 := make([]byte, 5760*2)
	opusDecMu.Lock()
	_, _, err := opusDec.Decode(opusFrame, pcm48)
	opusDecMu.Unlock()
	if err != nil {
		return nil
	}
	samples48 := len(pcm48) / 2
	if samples48 < 960 {
		return nil
	}
	// 48 kHz → 8 kHz：每 6 个样本取 1 个（单声道 S16LE）。
	outLen := samples48 / 6
	if outLen == 0 {
		return nil
	}
	out := make([]byte, outLen)
	for i := 0; i < outLen; i++ {
		off := i * 6 * 2
		sample := int16(int(pcm48[off]) | int(pcm48[off+1])<<8)
		out[i] = linearToMulaw(sample)
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
