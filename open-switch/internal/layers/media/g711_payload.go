package media

// G.711 conversion requires an explicitly negotiated static payload type.
func rtpPayloadToPCMU(pt uint8, payload []byte) []byte {
	switch pt {
	case 0:
		return payload
	case 8:
		return transcodeG711(8, 0, payload)
	default:
		return nil
	}
}
func pcmuPayloadToPCM(payload []byte) []int16 {
	out := make([]int16, len(payload))
	for i, v := range payload {
		out[i] = mulawToLinear(v)
	}
	return out
}
