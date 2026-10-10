package media

import (
	"fmt"
	"testing"
	"time"
)

func waitPlayout(t *testing.T, b *legPlayoutBuffer) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		b.mu.Lock()
		ready := b.anchored && !time.Now().Before(b.readyAt)
		b.mu.Unlock()
		if ready {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("playout never ready")
		}
		time.Sleep(time.Millisecond)
	}
}
func TestPlayoutPacketization(t *testing.T) {
	for _, n := range []int{80, 160, 240, 320} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			b := newLegPlayoutBuffer(defaultPlayoutCap)
			defer b.close()
			for i := 0; i < 4; i++ {
				pcm := make([]int16, n)
				for j := range pcm {
					pcm[j] = int16(1000 + i*n + j)
				}
				b.ingest(uint16(i), 1, uint32(i*n), pcm)
			}
			waitPlayout(t, b)
			for samples := 0; samples < 4*n; samples += 160 {
				frame := b.pullFrame()
				if len(frame) != 160 {
					t.Fatal("output framing")
				}
				for j, v := range frame {
					if v != int16(1000+samples+j) {
						t.Fatalf("sample truncated or duplicated: %d", v)
					}
				}
			}
		})
	}
}
func TestPlayoutFirstReorderAndWrap(t *testing.T) {
	b := newLegPlayoutBuffer(defaultPlayoutCap)
	defer b.close()
	a := make([]int16, 160)
	z := make([]int16, 160)
	for i := range a {
		a[i] = 1200
		z[i] = 2400
	}
	b.ingest(0, 1, 80, z)
	b.ingest(65535, 1, 0xffffffb0, a)
	waitPlayout(t, b)
	if b.pullFrame()[0] != 1200 || b.pullFrame()[0] != 2400 {
		t.Fatal("first reorder/timestamp wrap lost audio")
	}
	if rtpSampleIndex(1000, 1160) != -160 {
		t.Fatal("negative timestamp delta became a future epoch")
	}
}
func TestPlayoutReceivedSilenceIsNotLoss(t *testing.T) {
	b := newLegPlayoutBuffer(defaultPlayoutCap)
	defer b.close()
	a := make([]int16, 160)
	for i := range a {
		a[i] = 1000
	}
	b.ingest(1, 1, 0, a)
	b.ingest(2, 1, 160, make([]int16, 160))
	waitPlayout(t, b)
	b.pullFrame()
	for _, v := range b.pullFrame() {
		if v != 0 {
			t.Fatal("received silence concealed")
		}
	}
	if b.plcSamples != 0 {
		t.Fatal("PLC was invoked for real silence")
	}
}
func TestPlayoutDuplicateAndSSRCReset(t *testing.T) {
	b := newLegPlayoutBuffer(defaultPlayoutCap)
	defer b.close()
	a := make([]int16, 160)
	for i := range a {
		a[i] = 111
	}
	b.ingest(1, 1, 0, a)
	b.ingest(1, 1, 0, a)
	waitPlayout(t, b)
	if b.pullFrame()[0] != 111 {
		t.Fatal("initial audio")
	}
	b.ingest(1, 2, 10000, a)
	waitPlayout(t, b)
	if b.pullFrame()[0] != 111 {
		t.Fatal("SSRC did not reset stream")
	}
}
