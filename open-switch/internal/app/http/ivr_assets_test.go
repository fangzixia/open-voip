package http

import (
	"encoding/binary"
	"testing"
)

func testWAV(channels, rate uint32) []byte {
	raw := make([]byte, 46)
	copy(raw[:4], "RIFF")
	binary.LittleEndian.PutUint32(raw[4:8], uint32(len(raw)-8))
	copy(raw[8:12], "WAVE")
	copy(raw[12:16], "fmt ")
	binary.LittleEndian.PutUint32(raw[16:20], 16)
	binary.LittleEndian.PutUint16(raw[20:22], 1)
	binary.LittleEndian.PutUint16(raw[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(raw[24:28], rate)
	binary.LittleEndian.PutUint32(raw[28:32], rate*channels*2)
	binary.LittleEndian.PutUint16(raw[32:34], uint16(channels*2))
	binary.LittleEndian.PutUint16(raw[34:36], 16)
	copy(raw[36:40], "data")
	binary.LittleEndian.PutUint32(raw[40:44], 2)
	return raw
}

func TestValidPromptWAV(t *testing.T) {
	for _, rate := range []uint32{8000, 16000} {
		if !validPromptWAV(testWAV(1, rate)) {
			t.Fatalf("expected mono PCM %d Hz to be valid", rate)
		}
	}
	if validPromptWAV(testWAV(2, 8000)) || validPromptWAV(testWAV(1, 44100)) {
		t.Fatal("unsupported channels or rate accepted")
	}
	if validPromptWAV(testWAV(1, 8000)[:40]) {
		t.Fatal("truncated data accepted")
	}
}
