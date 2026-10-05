package media

// preparePromptPCM 在 8 kHz 域裁剪前导静音并做短淡入；阈值不宜过高，否则会吃掉「您好」等轻声起音。
func preparePromptPCM(pcm []int16) []int16 {
	if len(pcm) == 0 {
		return pcm
	}
	const thresh = 120
	start := len(pcm)
	for i, s := range pcm {
		v := int(s)
		if v < 0 {
			v = -v
		}
		if v > thresh {
			start = i
			break
		}
	}
	if start == len(pcm) {
		start = 0
	}
	const preroll = 160 // 20 ms @ 8 kHz，保留起音前气息/辅音
	if start > preroll {
		start -= preroll
	} else {
		start = 0
	}
	pcm = pcm[start:]
	const fade = 48 // 6 ms
	const fadeFloor = 8192 // 约 25% 音量，避免首字被淡入压到听不见
	for i := 0; i < fade && i < len(pcm); i++ {
		g := fadeFloor + int32(i+1)*(32767-fadeFloor)/int32(fade)
		pcm[i] = int16(int32(pcm[i]) * g / 32767)
	}
	return pcm
}
