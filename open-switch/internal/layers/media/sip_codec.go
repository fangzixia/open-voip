package media

// sipAudioCodec 表示 SIP 腿协商的音频编解码。
type sipAudioCodec uint8

const (
	sipCodecPCMU sipAudioCodec = 0
	sipCodecPCMA sipAudioCodec = 8
	sipCodecG722 sipAudioCodec = 9
	sipCodecOpus sipAudioCodec = 111
)

func (c sipAudioCodec) payloadType() uint8 { return uint8(c) }

func (c sipAudioCodec) sampleRate() int {
	switch c {
	case sipCodecG722:
		return 16000
	case sipCodecOpus:
		return 48000
	default:
		return 8000
}
}

func (c sipAudioCodec) frameSamples20ms() int {
	switch c {
	case sipCodecG722:
		return 320
	case sipCodecOpus:
		return 960
	default:
		return 160
}
}

func (c sipAudioCodec) isG711() bool { return c == sipCodecPCMU || c == sipCodecPCMA }

func codecFromPT(pt uint8) sipAudioCodec {
	switch pt {
	case 8:
		return sipCodecPCMA
	case 9:
		return sipCodecG722
	case 111:
		return sipCodecOpus
	default:
		return sipCodecPCMU
	}
}
