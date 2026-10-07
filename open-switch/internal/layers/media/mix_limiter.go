package media

import "math"

const mixHeadroom = 0.85

func mixPCMFrames(frames map[string][]int16, skipLeg string) []int16 {
	return mixPCMFramesLimited(frames, skipLeg)
}

// mixPCMFramesLimited 多路 float 累加后经软限幅，再转 int16。
func mixPCMFramesLimited(frames map[string][]int16, skipLeg string) []int16 {
	n := mixInternalFrameSamples
	if n <= 0 {
		n = mixFrameSamples
	}
	buf := make([]float64, n)
	for legID, frame := range frames {
		if legID == skipLeg || len(frame) == 0 {
			continue
		}
		lim := len(frame)
		if lim > n {
			lim = n
		}
		for i := 0; i < lim; i++ {
			buf[i] += float64(frame[i])
		}
	}
	ceil := 32767.0 * mixHeadroom
	out := make([]int16, n)
	for i, v := range buf {
		out[i] = int16(softLimit(v, ceil))
	}
	return out
}

func softLimit(v, ceil float64) int16 {
	if v > ceil {
		v = ceil + (v-ceil)*0.15
	} else if v < -ceil {
		v = -ceil + (v+ceil)*0.15
	}
	if v > 32767 {
		v = 32767
	} else if v < -32768 {
		v = -32768
	}
	return int16(math.Round(v))
}
