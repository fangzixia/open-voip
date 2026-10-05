package media

// pcm48kToOpusFrame 将 20ms@48kHz 单声道 PCM 编码为 Opus（由 WebRTC 栈编码时可为空实现）。
func pcm48kToOpusFrame(pcm []int16) []byte {
	_ = pcm
	return nil
}
