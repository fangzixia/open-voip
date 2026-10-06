package media

// 录音与混音路径上的 PCM 重采样（与 open-call/aibot/realtime 算法对齐）。
// Opus 解码为 48k 后需降到录音率或 8k 混音轨时使用，避免简单抽点混叠。

// downsamplePCMTo8k 将 16-bit PCM 转为 8 kHz；16 kHz 采用相邻样本均值以降低混叠。
func downsamplePCMTo8k(pcm []int16, rate int) ([]int16, int) {
	if rate <= 0 || rate == 8000 {
		return pcm, 8000
	}
	if rate == 16000 {
		n := len(pcm) / 2
		out := make([]int16, n)
		for i := 0; i < n; i++ {
			j := 2 * i
			var sum int32
			var cnt int32
			for _, k := range []int{j - 1, j, j + 1, j + 2} {
				if k >= 0 && k < len(pcm) {
					sum += int32(pcm[k])
					cnt++
				}
			}
			if cnt > 0 {
				out[i] = int16(sum / cnt)
			}
		}
		return out, 8000
	}
	outRate := 8000
	if rate < outRate {
		return pcm, rate
	}
	return resamplePCM(pcm, rate, outRate), outRate
}

// resamplePCM 单声道重采样：降采样先低通再抽取，升采样线性插值。
func resamplePCM(pcm []int16, fromRate, toRate int) []int16 {
	if fromRate <= 0 || toRate <= 0 || len(pcm) == 0 || fromRate == toRate {
		return pcm
	}
	if fromRate > toRate {
		return decimateWithLowpass(pcm, fromRate, toRate)
	}
	return linearResample(pcm, fromRate, toRate)
}

// linearResample 线性插值重采样。
func linearResample(pcm []int16, fromRate, toRate int) []int16 {
	outLen := len(pcm) * toRate / fromRate
	if outLen <= 0 {
		return nil
	}
	out := make([]int16, outLen)
	for i := 0; i < outLen; i++ {
		srcPos := float64(i) * float64(fromRate) / float64(toRate)
		j := int(srcPos)
		frac := srcPos - float64(j)
		if j >= len(pcm)-1 {
			out[i] = pcm[len(pcm)-1]
			continue
		}
		a, b := float64(pcm[j]), float64(pcm[j+1])
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

// decimateWithLowpass 降采样前盒式低通，减轻混叠。
func decimateWithLowpass(in []int16, fromRate, toRate int) []int16 {
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
