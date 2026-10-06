package aibot

import (
	"context"
	"testing"
	"time"
)

func TestDecodeWAVMono(t *testing.T) {
	// 最小合法 WAV 头 + 少量 PCM（手工构造较繁琐，仅测空输入）。
	if _, _, err := decodeWAV(nil); err == nil {
		t.Fatal("expected error for empty wav")
	}
}

func TestResamplePCM(t *testing.T) {
	in := []int16{0, 100, 200, 300}
	out := resamplePCM16(in, 8000, 16000)
	if len(out) != 8 {
		t.Fatalf("expected 8 samples, got %d", len(out))
	}
}

func TestPlaySilenceToPeerNilPeer(t *testing.T) {
	if err := PlaySilenceToPeer(context.Background(), nil, 600*time.Millisecond); err != nil {
		t.Fatal(err)
	}
}
