// Package realtime 封装 OpenAI/通义 Realtime WebSocket 与音频工具（重采样等）。
package realtime

// ResamplePCM 单声道 PCM 重采样。
//
// 降采样（如 24k→8k）：先盒式低通抑制混叠，再按时间轴抽取，避免「每 N 点取 1」的发闷失真。
// 升采样：线性插值并限幅到 int16。
func ResamplePCM(in []int16, fromRate, toRate int) []int16 {
	if fromRate <= 0 || toRate <= 0 || len(in) == 0 || fromRate == toRate {
		return in
	}
	if fromRate > toRate {
		in = lowpassDecimate(in, fromRate, toRate)
		fromRate = toRate
		if len(in) == 0 {
			return nil
		}
	}
	return linearResample(in, fromRate, toRate)
}

// ResampleSimple 兼容旧名，行为同 ResamplePCM。
func ResampleSimple(in []int16, fromRate, toRate int) []int16 {
	return ResamplePCM(in, fromRate, toRate)
}

// linearResample 线性插值重采样（用于升采样或小幅比率变化）。
func linearResample(in []int16, fromRate, toRate int) []int16 {
	outLen := len(in) * toRate / fromRate
	if outLen <= 0 {
		return nil
	}
	out := make([]int16, outLen)
	for i := 0; i < outLen; i++ {
		srcPos := float64(i) * float64(fromRate) / float64(toRate)
		j := int(srcPos)
		frac := srcPos - float64(j)
		if j >= len(in)-1 {
			out[i] = in[len(in)-1]
			continue
		}
		a, b := float64(in[j]), float64(in[j+1])
		v := a + (b-a)*frac
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		out[i] = int16(v)
	}
	return out
}

// lowpassDecimate 降采样前做盒式低通（窗口约为 from/to 比率），再抽取到目标率。
func lowpassDecimate(in []int16, fromRate, toRate int) []int16 {
	if fromRate <= toRate {
		return in
	}
	ratio := fromRate / toRate
	if ratio < 2 {
		return linearResample(in, fromRate, toRate)
	}
	win := ratio
	if win > 12 {
		win = 12
	}
	filtered := make([]int16, len(in))
	for i := range in {
		var sum int64
		var n int
		for k := -win; k <= win; k++ {
			j := i + k
			if j < 0 || j >= len(in) {
				continue
			}
			sum += int64(in[j])
			n++
		}
		if n > 0 {
			filtered[i] = int16(sum / int64(n))
		}
	}
	outLen := len(filtered) * toRate / fromRate
	if outLen <= 0 {
		return nil
	}
	out := make([]int16, outLen)
	for i := range out {
		srcPos := float64(i) * float64(fromRate) / float64(toRate)
		j := int(srcPos)
		if j >= len(filtered) {
			j = len(filtered) - 1
		}
		out[i] = filtered[j]
	}
	return out
}
