package media

import "encoding/binary"

const mixFrameSamples = 160 // 20 ms @ 8 kHz

func pcmToPCMU(pcm []int16) []byte {
	out := make([]byte, len(pcm))
	for i, s := range pcm {
		out[i] = linearToMulaw(s)
	}
	return out
}

func pcm16ToBytes(pcm []int16) []byte {
	b := make([]byte, len(pcm)*2)
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(s))
	}
	return b
}
