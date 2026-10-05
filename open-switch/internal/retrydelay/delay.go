package retrydelay

import (
	"time"

	"github.com/cenkalti/backoff/v4"
)

// Exponential 返回第 attempt 次失败后的退避时长（attempt 从 1 起）。
func Exponential(attempt int, maxInterval time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if maxInterval <= 0 {
		maxInterval = time.Hour
	}
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 2 * time.Second
	b.Multiplier = 2
	b.RandomizationFactor = 0
	b.MaxInterval = maxInterval
	b.Reset()
	var d time.Duration
	for i := 0; i < attempt; i++ {
		d = b.NextBackOff()
	}
	return d
}
