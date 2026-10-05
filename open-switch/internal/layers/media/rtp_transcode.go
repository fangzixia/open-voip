package media

func (s *Service) rtpAudioToPCMU(rt *sipRTP, payloadType uint8, payload []byte) []byte {
	if payloadType == 9 && rt != nil {
		return s.g722RTPToPCMU(rt, payload)
	}
	return rtpPayloadToPCMU(payloadType, payload)
}

func (s *Service) g722RTPToPCMU(rt *sipRTP, payload []byte) []byte {
	if len(payload) == 0 {
		return nil
	}
	rt.mu.Lock()
	if rt.g722Dec == nil {
		rt.g722Dec, _ = newFFmpegG722Decoder(s.ffmpegPath)
	}
	dec := rt.g722Dec
	rt.mu.Unlock()
	if dec == nil {
		return nil
	}
	pcm16, err := dec.decodeFrame(payload)
	if err != nil || len(pcm16) == 0 {
		return nil
	}
	pcm8 := resamplePCM(pcm16, 16000, 8000)
	return pcmToG711(pcm8, 0)
}
