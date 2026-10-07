package media

import (
	"encoding/binary"
	"os"
	"sync"
	"time"
)

// pcmMix 按墙钟时间轴将多路音频混为 16-bit PCM，流式写入 WAV。
type pcmMix struct {
	mu      sync.Mutex
	started time.Time // 混音时间轴起点
	rate    int         // 目标采样率 Hz
	samples []int16     // 未刷盘的尾部缓冲
	file    *os.File
	written int // 已写入文件的样本数
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

// newPCMMix 创建混音器，rate 为主录音目标采样率（如 16000）。
func newPCMMix(rate int, started time.Time) *pcmMix {
	if rate <= 0 {
		rate = 8000
	}
	return &pcmMix{started: started, rate: rate}
}

// anchorAt 将混音时间轴对齐到实际可听内容起点（如 IVR 首帧出站），避免应答前空白与错位。
func (m *pcmMix) anchorAt(t time.Time) {
	if m == nil {
		return
	}
	m.started = t
	m.samples = nil
	if m.file != nil && m.written == 0 {
		_ = m.header(0)
	}
}

// addLinearAtSampleIdx 在录音采样下标处饱和相加（多方主混音）。
func (m *pcmMix) addLinearAtSampleIdx(idx int, samples []int16, sampleRate int) {
	if m == nil || len(samples) == 0 || sampleRate <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if sampleRate != m.rate {
		samples = resamplePCM(samples, sampleRate, m.rate)
	}
	if idx < 0 {
		idx = 0
	}
	if m.file != nil {
		m.flush(idx - m.written - m.rate)
		idx -= m.written
	}
	need := idx + len(samples)
	if cap(m.samples) < need {
		n := make([]int16, need, need*2)
		copy(n, m.samples)
		m.samples = n
	} else if len(m.samples) < need {
		m.samples = m.samples[:need]
	}
	for i, s := range samples {
		v := int32(m.samples[idx+i]) + int32(s)
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		m.samples[idx+i] = int16(v)
	}
}

// writeLinearAtSampleIdx 在录音采样下标处写入（单 leg 分轨，同流不叠加）。
func (m *pcmMix) writeLinearAtSampleIdx(idx int, samples []int16, sampleRate int) {
	if m == nil || len(samples) == 0 || sampleRate <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if sampleRate != m.rate {
		samples = resamplePCM(samples, sampleRate, m.rate)
	}
	if idx < 0 {
		idx = 0
	}
	if m.file != nil {
		m.flush(idx - m.written - m.rate)
		idx -= m.written
	}
	need := idx + len(samples)
	if cap(m.samples) < need {
		n := make([]int16, need, need*2)
		copy(n, m.samples)
		m.samples = n
	} else if len(m.samples) < need {
		m.samples = m.samples[:need]
	}
	for i, s := range samples {
		m.samples[idx+i] = s
	}
}

// addLinearPCM 将线性 PCM 混音到墙钟时间轴（用于 G.711 解码后或 Opus 解码后的高质量录音）。
//
// 用到达时间 at 映射到样本下标 idx，多路音频在同一时间窗口做饱和相加。
// 流式写盘时 flush 已落盘前缀，内存只保留约 1 秒滑动窗口，避免长通话 OOM。
func (m *pcmMix) addLinearPCM(samples []int16, sampleRate int, at time.Time) {
	if m == nil || len(samples) == 0 || sampleRate <= 0 {
		return
	}
	if sampleRate != m.rate {
		samples = resamplePCM(samples, sampleRate, m.rate)
	}
	idx := int(at.Sub(m.started).Seconds() * float64(m.rate))
	if idx < 0 {
		idx = 0
	}
	if m.file != nil {
		m.flush(idx - m.written - m.rate)
		idx -= m.written
	}
	need := idx + len(samples)
	if cap(m.samples) < need {
		n := make([]int16, need, need*2)
		copy(n, m.samples)
		m.samples = n
	} else if len(m.samples) < need {
		m.samples = m.samples[:need]
	}
	for i, s := range samples {
		v := int32(m.samples[idx+i]) + int32(s)
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		m.samples[idx+i] = int16(v)
	}
}

// add 将一帧 G.711 载荷按墙钟位置叠加进混音（兼容旧路径，新录音优先 addLinearPCM）。
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

// appendSequential 按媒体顺序追加样本（FS 式 tap 写盘，不用 RTP 时间轴下标）。
func (m *pcmMix) appendSequential(samples []int16) {
	if m == nil || len(samples) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = append(m.samples, samples...)
	if m.file != nil {
		for len(m.samples) > m.rate {
			m.flush(len(m.samples) - m.rate)
		}
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
