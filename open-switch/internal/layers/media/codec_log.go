package media

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"open-switch/internal/observability"
)

// offeredCodecsLabel 将 SDP offer 中的音频 PT 转为可读列表（用于日志）。
func (m sdpMedia) offeredCodecsLabel() string {
	if len(m.Types) == 0 {
		return "default_g711"
	}
	parts := make([]string, 0, len(m.Types))
	for _, t := range m.Types {
		switch t {
		case 0:
			parts = append(parts, "PCMU")
		case 8:
			parts = append(parts, "PCMA")
		case 9:
			parts = append(parts, "G722")
		case 101:
			parts = append(parts, "telephone-event")
		default:
			if m.opusPT >= 0 && t == m.opusPT {
				parts = append(parts, "OPUS")
			} else {
				parts = append(parts, fmt.Sprintf("PT%d", t))
			}
		}
	}
	return strings.Join(parts, ",")
}

func logCodecNegotiation(rtpSess *sipRTP, offer sdpMedia, codec sipAudioCodec, pt uint8, preferWB, preferOpus bool, audioProfile string, phase string) {
	if rtpSess == nil {
		return
	}
	name := codecName(codec)
	rate := codec.sampleRate()
	ctx := observability.WithFields(context.Background(), observability.Fields{
		CallID: rtpSess.callID,
		LegID:  rtpSess.legID,
	})
	observability.Event(ctx, "sip_rtp", "codec.negotiated", phase, "ok", "", time.Now(),
		"codec_negotiated", name,
		"payload_type", pt,
		"sample_rate_hz", rate,
		"offer_codecs", offer.offeredCodecsLabel(),
		"audio_profile", audioProfile,
		"prefer_wideband", preferWB,
		"prefer_opus", preferOpus,
	)
	slog.Info("SIP 音频编码已协商",
		"call_id", rtpSess.callID,
		"leg_id", rtpSess.legID,
		"codec_negotiated", name,
		"payload_type", pt,
		"sample_rate_hz", rate,
		"offer_codecs", offer.offeredCodecsLabel(),
		"audio_profile", audioProfile,
		"prefer_wideband", preferWB,
		"prefer_opus", preferOpus,
		"phase", phase,
	)
}
