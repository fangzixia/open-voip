package media

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

// resamplePCM 线性插值重采样（含降采样低通：先低通再抽取由调用方 rate 选择处理）。
func resamplePCM(pcm []int16, fromRate, toRate int) []int16 {
	if fromRate <= 0 || toRate <= 0 || len(pcm) == 0 || fromRate == toRate {
		return pcm
	}
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
		out[i] = int16(a + (b-a)*frac)
	}
	return out
}
