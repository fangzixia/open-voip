package media

import "time"

const rtpFrameDur = 20 * time.Millisecond

// paceFrame 按墙钟等待下一帧发送时刻，返回本帧相对计划时刻的迟到毫秒（用于观测）。
func paceFrame(next *time.Time) int {
	now := time.Now()
	if next.IsZero() {
		*next = now
	}
	if now.Before(*next) {
		time.Sleep(next.Sub(now))
		now = time.Now()
	}
	late := int(now.Sub(*next).Milliseconds())
	if late < 0 {
		late = 0
	}
	*next = next.Add(rtpFrameDur)
	if now.After(*next) {
		*next = now.Add(rtpFrameDur)
	}
	return late
}
