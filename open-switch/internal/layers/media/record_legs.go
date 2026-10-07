package media

import (
	"path/filepath"
	"strings"

	"github.com/pion/rtp"
)

// sanitizeLegID 将 leg UUID 转为安全文件名片段。
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

// ensureLegPCM 懒创建某通话腿的 16-bit 分轨混音器及对应 WAV 路径。
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

// writeHQAudio 解码 RTP 音频后写入主混音与各 leg 分轨（线性 PCM，非直接叠 G.711）。
//
// 主文件为多方混音；分轨文件仅含该 leg 的解码 PCM，便于质检区分主叫/坐席/AI。
// 解码在 G.711 之前完成，避免「编解码后再叠轨」带来的额外窄带损失。
func (rec *recorder) writeHQAudio(legID string, mime string, pkt *rtp.Packet) {
	if rec.pcm == nil || pkt == nil {
		return
	}
	samples, sr := decodeRTPAudioForLeg(legID, pkt.PayloadType, mime, pkt.Payload)
	if len(samples) == 0 || sr <= 0 {
		return
	}
	if rec.timelines == nil {
		rec.timelines = newRecordTimelineStore()
	}
	idx64 := rec.timelines.sampleIndex(legID, pkt.SSRC, pkt.Timestamp, len(samples), sr)
	if idx64 < 0 {
		return
	}
	// 8k/16k/48k 源 PCM 重采样到录音率后按 RTP 下标落盘。
	scaleNum := rec.pcm.rate
	scaleDen := sr
	idx := int(idx64 * int64(scaleNum) / int64(scaleDen))
	if rec.pcmAsync != nil {
		rec.pcmAsync.enqueue(func() {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			rec.appendHQAudioAt(legID, idx, samples, sr)
		})
		return
	}
	rec.appendHQAudioAt(legID, idx, samples, sr)
}

// appendHQAudioAt 在已持有 rec.mu 或单线程异步 worker 内写入混音与分轨。
func (rec *recorder) appendHQAudioAt(legID string, idx int, samples []int16, sr int) {
	if rec.pcm == nil {
		return
	}
	rec.pcm.addLinearAtSampleIdx(idx, samples, sr)
	rec.bytes = rec.pcm.byteSize()
	if leg := rec.ensureLegPCM(legID, rec.pcm.rate); leg != nil {
		leg.writeLinearAtSampleIdx(idx, samples, sr)
	}
}
