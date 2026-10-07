package media

import (
	"encoding/json"
)

type webrtcStatsSummary struct {
	Inbound  []rtpLegStats `json:"inbound"`
	Outbound []rtpLegStats `json:"outbound"`
}

type rtpLegStats struct {
	SSRC                  uint32  `json:"ssrc"`
	Kind                  string  `json:"kind"`
	MimeType              string  `json:"mime_type"`
	PacketsReceived       uint64  `json:"packets_received,omitempty"`
	PacketsSent           uint64  `json:"packets_sent,omitempty"`
	PacketsLost           int64   `json:"packets_lost,omitempty"`
	Jitter                float64 `json:"jitter,omitempty"`
	RTTMs                 float64 `json:"rtt_ms,omitempty"`
	ConcealedSamples      uint64  `json:"concealed_samples,omitempty"`
	JitterBufferDelay     float64 `json:"jitter_buffer_delay,omitempty"`
	JitterBufferEmitted   uint64  `json:"jitter_buffer_emitted_count,omitempty"`
}

func summarizeWebRTCStats(raw []byte) (webrtcStatsSummary, float64) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return webrtcStatsSummary{}, 0
	}
	var rttMs float64
	sum := webrtcStatsSummary{}
	for _, v := range root {
		var row map[string]any
		if err := json.Unmarshal(v, &row); err != nil {
			continue
		}
		typ, _ := row["type"].(string)
		switch typ {
		case "candidate-pair":
			if nominated, _ := row["nominated"].(bool); nominated {
				if rtt, ok := row["currentRoundTripTime"].(float64); ok {
					rttMs = rtt * 1000
				}
			}
		case "inbound-rtp":
			sum.Inbound = append(sum.Inbound, parseRTPLeg(row, true))
		case "outbound-rtp":
			sum.Outbound = append(sum.Outbound, parseRTPLeg(row, false))
		}
	}
	return sum, rttMs
}

func parseRTPLeg(row map[string]any, inbound bool) rtpLegStats {
	st := rtpLegStats{Kind: str(row["kind"]), MimeType: str(row["mimeType"])}
	if v, ok := row["ssrc"].(float64); ok {
		st.SSRC = uint32(v)
	}
	if inbound {
		st.PacketsReceived = num(row["packetsReceived"])
		st.PacketsLost = int64(num(row["packetsLost"]))
		if j, ok := row["jitter"].(float64); ok {
			st.Jitter = j
		}
		st.ConcealedSamples = num(row["concealedSamples"])
		if d, ok := row["jitterBufferDelay"].(float64); ok {
			st.JitterBufferDelay = d
		}
		st.JitterBufferEmitted = num(row["jitterBufferEmittedCount"])
	} else {
		st.PacketsSent = num(row["packetsSent"])
	}
	return st
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) uint64 {
	switch t := v.(type) {
	case float64:
		if t < 0 {
			return 0
		}
		return uint64(t)
	default:
		return 0
	}
}
