package media

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/pion/rtp"
)

func sanitizeLegID(legID string) string {
	legID = strings.TrimSpace(legID)
	if legID == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range legID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "leg"
	}
	if len(out) > 48 {
		return out[:48]
	}
	return out
}

func (rec *recorder) ensureLegPCM(legID string, rate int) *pcmMix {
	if rec == nil || rec.legPCM == nil {
		return nil
	}
	if m, ok := rec.legPCM[legID]; ok {
		return m
	}
	dir := filepath.Dir(rec.path)
	safe := sanitizeLegID(legID)
	legPath := filepath.Join(dir, strings.TrimSuffix(filepath.Base(rec.path), filepath.Ext(rec.path))+"-leg-"+safe+".wav")
	m := newPCMMix(rate, rec.started)
	if err := m.startFile(legPath); err != nil {
		return nil
	}
	rec.legPCM[legID] = m
	if rec.legPaths == nil {
		rec.legPaths = map[string]string{}
	}
	rec.legPaths[legID] = legPath
	return m
}

func (rec *recorder) writeHQAudio(legID string, mime string, pkt *rtp.Packet) {
	if rec.pcm == nil || pkt == nil {
		return
	}
	samples, sr := decodeRTPAudio(pkt.PayloadType, mime, pkt.Payload)
	if len(samples) == 0 || sr <= 0 {
		return
	}
	at := time.Now()
	rec.pcm.addLinearPCM(samples, sr, at)
	rec.bytes = rec.pcm.byteSize()
	if leg := rec.ensureLegPCM(legID, rec.pcm.rate); leg != nil {
		leg.addLinearPCM(samples, sr, at)
	}
}
