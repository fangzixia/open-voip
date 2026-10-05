package media

import "github.com/zaf/g711"

// transcodeG711 在 PCMU(PT=0) 与 PCMA(PT=8) 之间转换；相同 PT 原样返回。
func transcodeG711(fromPT, toPT uint8, payload []byte) []byte {
	if len(payload) == 0 || fromPT == toPT {
		return payload
	}
	if fromPT == 0 && toPT == 8 {
		return g711.Ulaw2Alaw(payload)
	}
	if fromPT == 8 && toPT == 0 {
		return g711.Alaw2Ulaw(payload)
	}
	return payload
}

func linearToMulaw(sample int16) byte { return g711.EncodeUlawFrame(sample) }

func mulawToLinear(u byte) int16 { return g711.DecodeUlawFrame(u) }

func alawToLinear(a byte) int16 { return g711.DecodeAlawFrame(a) }
