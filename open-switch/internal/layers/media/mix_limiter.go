package media

const mixHeadroom = 0.85

func mixPCMFrames(frames map[string][]int16, skip string) []int16 {
	return mixPCMFramesLimited(frames, skip)
}

// Reserve headroom before summing. Silence is still an eligible source and
// cannot increase the gain of the other sources unexpectedly.
func mixPCMFramesLimited(frames map[string][]int16, skip string) []int16 {
	out := make([]int16, mixFrameSamples)
	count := 0
	for id, frame := range frames {
		if id != skip && len(frame) > 0 {
			count++
		}
	}
	if count == 0 {
		return out
	}
	gain := mixHeadroom / float64(count)
	for i := range out {
		var v float64
		for id, frame := range frames {
			if id != skip && i < len(frame) {
				v += float64(frame[i]) * gain
			}
		}
		out[i] = int16(v)
	}
	return out
}
