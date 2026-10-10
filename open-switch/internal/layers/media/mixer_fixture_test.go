package media

// Feed exact PCM only in sample-position tests; the production entry accepts
// negotiated G.711 RTP and decodes after the third-party jitter buffer.
func (m *scheduledRoomMixer) ingest(id string, seq uint16, ts, ssrc uint32, rate int, pcm []int16) {
	if rate != 8000 {
		panic("sample-position fixture requires 8 kHz")
	}
	m.mu.Lock()
	b := m.legs[id]
	if b == nil {
		b = newLegPlayoutBuffer(defaultPlayoutCap)
		m.legs[id] = b
	}
	m.mu.Unlock()
	b.ingest(seq, ssrc, ts, pcm)
}
