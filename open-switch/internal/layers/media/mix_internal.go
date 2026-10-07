package media

const mixInternalRate = mixClockRate // 窄带统一 8 kHz 混音工作区

const mixInternalFrameSamples = mixFrameSamples // 160 @ 20ms

func pcmToMixInternal(pcm []int16, sampleRate int) []int16 {
	if len(pcm) == 0 {
		return nil
	}
	if sampleRate <= 0 {
		sampleRate = mixClockRate
	}
	if sampleRate == mixInternalRate {
		return pcm
	}
	return resamplePCM(pcm, sampleRate, mixInternalRate)
}

func rtpClockRate(payloadType uint8, mime string) int {
	_ = payloadType
	_ = mime
	return mixClockRate
}

func tsToMixInternal(ts uint32, clockRate int) uint32 {
	if clockRate <= 0 || clockRate == mixInternalRate {
		return ts
	}
	return uint32(uint64(ts) * uint64(mixInternalRate) / uint64(clockRate))
}

func mixInternalTo8k(pcm []int16) []int16 {
	if len(pcm) == 0 {
		return pcm
	}
	out, _ := downsamplePCMTo8k(pcm, mixInternalRate)
	return out
}
