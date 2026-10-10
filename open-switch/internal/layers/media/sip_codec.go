package media

// sipAudioCodec 表示 SIP 腿协商的音频编解码。
type sipAudioCodec uint8

const (
	sipCodecPCMU    sipAudioCodec = 0
	sipCodecPCMA    sipAudioCodec = 8
	sipCodecUnknown sipAudioCodec = 255
)

func (c sipAudioCodec) payloadType() uint8 { return uint8(c) }

func (c sipAudioCodec) sampleRate() int {
	switch c {
	case sipCodecPCMU, sipCodecPCMA:
		return 8000
	default:
		return 0
	}
}

func codecFromPT(pt uint8) sipAudioCodec {
	switch pt {
	case 0:
		return sipCodecPCMU
	case 8:
		return sipCodecPCMA
	default:
		return sipCodecUnknown
	}
}
