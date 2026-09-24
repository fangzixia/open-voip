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
	file    *os.File
	written int
	err     error
}

// startFile 启用有界流式写入：混音缓冲仅保留约一秒样本。
func (m *pcmMix) startFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o640)
	if err != nil {
		return err
	}
	m.file = f
	return m.header(0)
}

func (m *pcmMix) header(samples int) error {
	b := make([]byte, 44)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+samples*2))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], uint32(m.rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(m.rate*2))
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(samples*2))
	_, err := m.file.WriteAt(b, 0)
	return err
}

func (m *pcmMix) flush(count int) {
	if m.file == nil || m.err != nil || count <= 0 {
		return
	}
	for count > 0 {
		n := count
		if n > m.rate {
			n = m.rate
		}
		b := make([]byte, n*2)
		used := n
		if used > len(m.samples) {
			used = len(m.samples)
		}
		for i := 0; i < used; i++ {
			binary.LittleEndian.PutUint16(b[i*2:], uint16(m.samples[i]))
		}
		if _, m.err = m.file.WriteAt(b, int64(44+m.written*2)); m.err != nil {
			return
		}
		m.samples = m.samples[used:]
		m.written += n
		count -= n
	}
	m.err = m.header(m.written)
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
	if m.file != nil {
		m.flush(idx - m.written - m.rate)
		idx -= m.written
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
	return int64(44 + (m.written+len(m.samples))*2)
}

func (m *pcmMix) writeWAV(path string) (int64, error) {
	if m == nil {
		return 0, nil
	}
	if m.file != nil {
		m.flush(len(m.samples))
		err := m.err
		if err == nil {
			err = m.file.Sync()
		}
		if e := m.file.Close(); err == nil {
			err = e
		}
		m.file = nil
		return int64(44 + m.written*2), err
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
