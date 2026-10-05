package media

import (
	"sync"

	"github.com/pion/rtp"
)

const mixFrameSamples = 160 // 20 ms @ 8 kHz

// roomMixer 在多方会议中将各腿最近一帧 PCM 混音后下发（不含本腿，避免回声）。
type roomMixer struct {
	mu     sync.Mutex
	frames map[string][]int16
	seq    map[string]uint16
	ts     map[string]uint32
}

func newRoomMixer() *roomMixer {
	return &roomMixer{frames: map[string][]int16{}, seq: map[string]uint16{}, ts: map[string]uint32{}}
}

func (m *roomMixer) ingest(fromLeg string, pcmu []byte) {
	if m == nil || len(pcmu) == 0 {
		return
	}
	frame := pcmuPayloadToPCM(pcmu)
	if len(frame) > mixFrameSamples {
		frame = frame[:mixFrameSamples]
	}
	m.mu.Lock()
	m.frames[fromLeg] = frame
	m.mu.Unlock()
}

func (m *roomMixer) mixExcept(skipLeg string) []int16 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]int16, mixFrameSamples)
	for leg, frame := range m.frames {
		if leg == skipLeg || len(frame) == 0 {
			continue
		}
		n := len(frame)
		if n > mixFrameSamples {
			n = mixFrameSamples
		}
		for i := 0; i < n; i++ {
			v := int32(out[i]) + int32(frame[i])
			if v > 32767 {
				v = 32767
			} else if v < -32768 {
				v = -32768
			}
			out[i] = int16(v)
		}
	}
	return out
}

func pcmToPCMU(pcm []int16) []byte {
	out := make([]byte, len(pcm))
	for i, s := range pcm {
		out[i] = linearToMulaw(s)
	}
	return out
}

func (m *roomMixer) nextRTP(legID string, pcmu []byte) *rtp.Packet {
	m.mu.Lock()
	seq := m.seq[legID]
	ts := m.ts[legID]
	m.seq[legID] = seq + 1
	m.ts[legID] = ts + uint32(len(pcmu))
	m.mu.Unlock()
	return &rtp.Packet{
		Header: rtp.Header{Version: 2, PayloadType: 0, SequenceNumber: seq, Timestamp: ts, SSRC: 1},
		Payload: pcmu,
	}
}
