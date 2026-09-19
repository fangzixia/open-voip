package media

import (
	"encoding/binary"
	"os"
	"time"
)

// pcmMix 将 G.711 按墙钟混音为 16-bit PCM，供 WAV 落盘。
type pcmMix struct {
	started time.Time
	rate    int
	samples []int16
}

func newPCMMix(rate int, started time.Time) *pcmMix {
	if rate <= 0 {
		rate = 8000
	}
	return &pcmMix{started: started, rate: rate}
}

func (m *pcmMix) add(payloadType uint8, payload []byte) {
	if m == nil || len(payload) == 0 {
		return
	}
	idx := int(time.Since(m.started).Seconds() * float64(m.rate))
	if idx < 0 {
		idx = 0
	}
	need := idx + len(payload)
	if cap(m.samples) < need {
		n := make([]int16, need, need*2)
		copy(n, m.samples)
		m.samples = n
	} else if len(m.samples) < need {
		m.samples = m.samples[:need]
	}
	for i, b := range payload {
		s := mulawToLinear(b)
		if payloadType == 8 {
			s = alawToLinear(b)
		}
		v := int32(m.samples[idx+i]) + int32(s)
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		m.samples[idx+i] = int16(v)
	}
}

func (m *pcmMix) byteSize() int64 {
	if m == nil {
		return 0
	}
	return int64(44 + len(m.samples)*2)
}

func (m *pcmMix) writeWAV(path string) (int64, error) {
	if m == nil {
		return 0, nil
	}
	dataBytes := len(m.samples) * 2
	buf := make([]byte, 44+dataBytes)
	copy(buf[0:], []byte("RIFF"))
	binary.LittleEndian.PutUint32(buf[4:], uint32(36+dataBytes))
	copy(buf[8:], []byte("WAVEfmt "))
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1)
	binary.LittleEndian.PutUint16(buf[22:], 1)
	binary.LittleEndian.PutUint32(buf[24:], uint32(m.rate))
	binary.LittleEndian.PutUint32(buf[28:], uint32(m.rate*2))
	binary.LittleEndian.PutUint16(buf[32:], 2)
	binary.LittleEndian.PutUint16(buf[34:], 16)
	copy(buf[36:], []byte("data"))
	binary.LittleEndian.PutUint32(buf[40:], uint32(dataBytes))
	for i, s := range m.samples {
		binary.LittleEndian.PutUint16(buf[44+i*2:], uint16(s))
	}
	if err := os.WriteFile(path, buf, 0o640); err != nil {
		return 0, err
	}
	return int64(len(buf)), nil
}

func mulawToLinear(u byte) int16 {
	u = ^u
	sign := u & 0x80
	exponent := (u >> 4) & 0x07
	mantissa := int16(u & 0x0f)
	sample := ((mantissa << 3) + 0x84) << exponent
	sample -= 0x84
	if sign != 0 {
		return -sample
	}
	return sample
}

func alawToLinear(a byte) int16 {
	a ^= 0x55
	t := int16(a&0x0f) << 4
	seg := (a & 0x70) >> 4
	switch {
	case seg == 0:
		t += 8
	case seg == 1:
		t += 0x108
	default:
		t += 0x108
		t <<= seg - 1
	}
	if a&0x80 != 0 {
		return t
	}
	return -t
}
