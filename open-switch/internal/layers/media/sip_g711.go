package media

// transcodeG711 在 PCMU(PT=0) 与 PCMA(PT=8) 之间按样本查表转换；相同 PT 原样返回。
func transcodeG711(fromPT, toPT uint8, payload []byte) []byte {
	if len(payload) == 0 || fromPT == toPT {
		return payload
	}
	if (fromPT != 0 && fromPT != 8) || (toPT != 0 && toPT != 8) {
		return payload
	}
	out := make([]byte, len(payload))
	if fromPT == 0 && toPT == 8 {
		for i, b := range payload {
			out[i] = linearToAlaw(mulawToLinear(b))
		}
		return out
	}
	for i, b := range payload {
		out[i] = linearToMulaw(alawToLinear(b))
	}
	return out
}

func linearToAlaw(sample int16) byte {
	const (
		clip  = 32635
		scale = 0x55
	)
	sign := byte(0)
	if sample < 0 {
		sign = 0x80
		sample = -sample
		if sample < 0 {
			sample = clip
		}
	}
	if sample > clip {
		sample = clip
	}
	sample += 8
	var exp byte
	for exp = 7; exp > 0; exp-- {
		if sample&(0x0100<<exp) != 0 {
			break
		}
	}
	mant := byte((sample >> (exp + 3)) & 0x0f)
	return (sign | (exp << 4) | mant) ^ scale
}
