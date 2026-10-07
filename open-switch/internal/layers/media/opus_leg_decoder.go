package media

import (
	"sync"

	"github.com/pion/opus"
)

type opusLegDecoders struct {
	mu    sync.Mutex
	dec   map[string]opus.Decoder
	last  map[string][]int16
}

var (
	legOpusMix   opusLegDecoders
	legOpusRecord opusLegDecoders
)

func (p *opusLegDecoders) decode(legID string, payload []byte, out []int16) (int, error) {
	if len(payload) == 0 {
		return 0, nil
	}
	p.mu.Lock()
	if p.dec == nil {
		p.dec = map[string]opus.Decoder{}
	}
	dec, ok := p.dec[legID]
	if !ok {
		var err error
		dec, err = opus.NewDecoderWithOutput(48000, 1)
		if err != nil {
			p.mu.Unlock()
			return 0, err
		}
		p.dec[legID] = dec
	}
	p.mu.Unlock()
	n, err := dec.DecodeToInt16(payload, out)
	if err != nil || n <= 0 {
		p.mu.Lock()
		prev := append([]int16(nil), p.last[legID]...)
		p.mu.Unlock()
		if len(prev) > 0 {
			if len(prev) > len(out) {
				prev = prev[:len(out)]
			}
			copy(out, prev)
			return len(prev), nil
		}
		return n, err
	}
	p.mu.Lock()
	if p.last == nil {
		p.last = map[string][]int16{}
	}
	p.last[legID] = append([]int16(nil), out[:n]...)
	p.mu.Unlock()
	return n, err
}

func rtpPayloadToPCMUForLeg(legID string, payloadType uint8, payload []byte) []byte {
	switch payloadType {
	case 0:
		return payload
	case 8:
		return transcodeG711(8, 0, payload)
	case opusPayloadType:
		return opusLegToPCMU(legID, payload)
	default:
		if payloadType >= 96 {
			return opusLegToPCMU(legID, payload)
		}
		return nil
	}
}

func opusLegToPCMU(legID string, opusFrame []byte) []byte {
	if len(opusFrame) == 0 {
		return nil
	}
	pcm48 := make([]int16, 5760)
	n, err := legOpusMix.decode(legID, opusFrame, pcm48)
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

func decodeRTPAudioForLeg(legID string, payloadType uint8, mime string, payload []byte) ([]int16, int) {
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
		if payloadType == opusPayloadType || mime == "audio/opus" || payloadType >= 96 {
			pcm48 := make([]int16, 5760)
			n, err := legOpusRecord.decode(legID, payload, pcm48)
			if err != nil || n <= 0 {
				return nil, 0
			}
			return pcm48[:n], 48000
		}
		return nil, 0
	}
}
