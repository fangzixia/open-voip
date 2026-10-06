package aibot

import (
	"context"
	"sync"
	"time"
)

// pcmDownlinkPacer 将 PCM 按 20ms/帧节拍写入 WebRTC，避免 Realtime 突发送样导致电话侧杂音。
type pcmDownlinkPacer struct {
	peer *PeerSession
	mu   sync.Mutex
	buf  []int16
}

// newPCMDownlinkPacer 创建下行节拍器，与电话 20 ms 帧对齐。
func newPCMDownlinkPacer(peer *PeerSession) *pcmDownlinkPacer {
	return &pcmDownlinkPacer{peer: peer}
}

// Push 追加待发送 PCM（由 Realtime 回调线程写入）。
func (p *pcmDownlinkPacer) Push(samples []int16) {
	if len(samples) == 0 {
		return
	}
	p.mu.Lock()
	p.buf = append(p.buf, samples...)
	p.mu.Unlock()
}

// Run 按 20 ms 节拍从缓冲区取 160 样本写入 WebRTC。
func (p *pcmDownlinkPacer) Run(ctx context.Context) {
	const frame = 160
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.mu.Lock()
			if len(p.buf) == 0 {
				p.mu.Unlock()
				continue
			}
			var chunk []int16
			if len(p.buf) >= frame {
				chunk = p.buf[:frame]
				p.buf = p.buf[frame:]
			} else {
				// 不足 160 样本时用静音补齐一帧，避免尾音被截断；WritePCMU8k 仍按整帧发 RTP。
				chunk = make([]int16, frame)
				copy(chunk, p.buf)
				p.buf = nil
			}
			p.mu.Unlock()
			_ = p.peer.WritePCMU8k(chunk)
		}
	}
}
