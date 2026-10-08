package media

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-audio/wav"
	"github.com/google/uuid"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"open-switch/internal/scope"
)

func (s *Service) resolvePrompt(ctx context.Context, path string) string {
	_ = ctx
	if filepath.Ext(path) != ".wav" {
		return ""
	}
	if _, err := uuid.Parse(strings.TrimSuffix(path, ".wav")); err != nil {
		return ""
	}
	return filepath.Join(s.audioRecDir, "prompts", scope.AssetNamespace(), path)
}

func (s *Service) playWaitingTone(callID, targetLegID string, loop bool, seq uint64) {
	prime := true
	for {
		s.playToneToRoom(callID, targetLegID, time.Second, seq, prime)
		prime = false
		if !loop {
			return
		}
		if !s.waitPromptInterval(callID, 3*time.Second, seq) {
			return
		}
	}
}

func (s *Service) waitPromptInterval(callID string, duration time.Duration, seq uint64) bool {
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		room := s.getRoom(callID)
		if room == nil || room.promptSeq.Load() != seq {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

func (s *Service) playPCMToRoom(callID, targetLegID string, pcm []int16, rate int, loop bool, seq uint64) {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("IVR 播放异常退出", "call_id", callID, "err", err)
		}
	}()
	ctx := context.Background()
	var rtpSeq uint16
	var rtpTS uint32
	const rtpSSRC = 0x49565231
	var nextSend time.Time
	var sent, lateTotal int
	s.primeSIPPrompt(ctx, callID, targetLegID, seq, &rtpSeq, &rtpTS, rtpSSRC, &nextSend, &lateTotal)

	needG722 := s.roomNeedsG722(callID)
	var g722Frames [][]byte
	if needG722 {
		pcm16 := pcm
		if rate != 16000 {
			pcm16 = resamplePCM(pcm, rate, 16000)
		}
		g722Frames, _ = pcm16kToG722Frames(ctx, s.ffmpegPath, preparePromptPCM(pcm16))
	}
	pcm, rate = downsamplePCMTo8k(pcm, rate)
	if rate <= 0 {
		rate = 8000
	}
	pcm = preparePromptPCM(pcm)
	frames := splitPCMFrames(pcm, 160)
	if len(frames) == 0 && len(g722Frames) == 0 {
		s.playToneToRoom(callID, targetLegID, time.Second, seq, true)
		return
	}
	n := len(frames)
	if len(g722Frames) > n {
		n = len(g722Frames)
	}
	codecLabel := "G711"
	if needG722 && len(g722Frames) > 0 {
		codecLabel = "G722"
	}
	for {
		for i := 0; i < n; i++ {
			var frame []int16
			if i < len(frames) {
				frame = frames[i]
			} else {
				frame = make([]int16, 160)
			}
			var g722 []byte
			if i < len(g722Frames) {
				g722 = g722Frames[i]
			}
			mulaw := pcmToG711(frame, 0)
			if !s.playOnePromptFrame(callID, targetLegID, seq, &rtpSeq, &rtpTS, rtpSSRC, mulaw, g722, &nextSend, &lateTotal) {
				s.emitPromptPlaySummary(ctx, callID, sent, lateTotal, codecLabel, loop)
				return
			}
			sent++
		}
		if !loop {
			s.emitPromptPlaySummary(ctx, callID, sent, lateTotal, codecLabel, loop)
			return
		}
	}
}

func (s *Service) playOnePromptFrame(callID, targetLegID string, seq uint64, rtpSeq *uint16, rtpTS *uint32, ssrc uint32, mulaw []byte, g722 []byte, nextSend *time.Time, lateTotal *int) bool {
	room := s.getRoom(callID)
	if room == nil {
		return false
	}
	gain, cont := room.promptGainAndContinue(seq)
	if !cont && gain == 0 {
		return false
	}
	if len(mulaw) > 0 {
		mulaw = scaleMulawFrame(mulaw, gain)
	}
	room.mu.RLock()
	rec := room.rec
	type peerMOH struct {
		samp *webrtc.TrackLocalStaticSample
	}
	peers := make([]peerMOH, 0, len(room.peers))
	for legID, p := range room.peers {
		if targetLegID != "" && legID != targetLegID {
			continue
		}
		if p.audioSamp != nil {
			peers = append(peers, peerMOH{p.audioSamp})
		}
	}
	rtps := make([]*sipRTP, 0, len(room.sipRTP))
	for rt := range room.sipRTP {
		if targetLegID != "" && rt.legID != targetLegID {
			continue
		}
		rtps = append(rtps, rt)
	}
	room.mu.RUnlock()
	for _, p := range peers {
		if len(mulaw) > 0 && p.samp != nil {
			_ = p.samp.WriteSample(media.Sample{Data: mulaw, Duration: rtpFrameDur})
		}
	}
	hdr := rtp.Header{Version: 2, SequenceNumber: *rtpSeq, Timestamp: *rtpTS, SSRC: ssrc}
	for _, rt := range rtps {
		if rt.currentCodec() == sipCodecG722 && len(g722) > 0 {
			h := hdr
			h.PayloadType = 9
			rt.writeRTPPacket(&rtp.Packet{Header: h, Payload: g722})
		} else if len(mulaw) > 0 {
			pt := rt.currentPT()
			h := hdr
			h.PayloadType = pt
			rt.writeRTPPacket(&rtp.Packet{Header: h, Payload: pcmToG711(pcmuPayloadToPCM(mulaw), pt)})
		}
	}
	if len(mulaw) > 0 {
		recordPromptToMix(rec, mulaw)
	}
	*lateTotal += paceFrame(nextSend)
	*rtpSeq++
	*rtpTS += 160
	return cont
}

func splitPCMFrames(pcm []int16, samples int) [][]int16 {
	if samples <= 0 {
		return nil
	}
	out := make([][]int16, 0, len(pcm)/samples+1)
	for i := 0; i < len(pcm); i += samples {
		frame := make([]int16, samples)
		for j := 0; j < samples; j++ {
			idx := i + j
			if idx < len(pcm) {
				frame[j] = pcm[idx]
			}
		}
		out = append(out, frame)
	}
	return out
}

func readPCMWav(path string) ([]int16, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()
	dec := wav.NewDecoder(f)
	if !dec.IsValidFile() {
		return nil, 0, io.ErrUnexpectedEOF
	}
	buf, err := dec.FullPCMBuffer()
	if err != nil {
		return nil, 0, err
	}
	if dec.BitDepth != 16 || dec.NumChans < 1 {
		return nil, 0, io.ErrUnexpectedEOF
	}
	ch := int(dec.NumChans)
	pcm := make([]int16, 0, len(buf.Data)/ch)
	for i := 0; i < len(buf.Data); i += ch {
		var sum int64
		for j := 0; j < ch && i+j < len(buf.Data); j++ {
			sum += int64(buf.Data[i+j])
		}
		pcm = append(pcm, int16(sum/int64(ch)))
	}
	if len(pcm) == 0 {
		return nil, 0, io.ErrUnexpectedEOF
	}
	return pcm, int(dec.SampleRate), nil
}
