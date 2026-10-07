package media

// encodePCMUForSIPLeg 将 8 kHz 线性 PCM（PCMU 域）编码为协商 codec 的 RTP 载荷（窄带仅 G.711）。
func (s *Service) encodePCMUForSIPLeg(rt *sipRTP, pcmu []byte) ([]byte, uint8) {
	if rt == nil || len(pcmu) == 0 {
		return pcmu, 0
	}
	rt.mu.Lock()
	codec := rt.remoteCodec
	pt := rt.remotePT
	rt.mu.Unlock()
	switch codec {
	case sipCodecPCMA:
		pcm8 := pcmuPayloadToPCM(pcmu)
		return pcmToG711(pcm8, 8), pt
	default:
		return pcmu, pt
	}
}
