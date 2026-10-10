package media

import (
	"errors"
	"sync"
)

type pcmConverter struct {
	mu        sync.Mutex
	resampler *streamingResampler
	closed    bool
	pending   []int16
}

func (c *pcmConverter) convert(pcm []int16, from, to int) ([]int16, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("PCM converter closed")
	}
	if from == to {
		return pcm, nil
	}
	if c.resampler == nil {
		r, e := newStreamingResampler(from, to, false)
		if e != nil {
			return nil, e
		}
		c.resampler = r
	}
	out, e := c.resampler.process(pcm, false)
	if e != nil {
		return nil, e
	}
	c.pending = append(c.pending, out...)
	n := len(c.pending) / (to / 50) * (to / 50)
	complete := append([]int16(nil), c.pending[:n]...)
	c.pending = append(c.pending[:0], c.pending[n:]...)
	return complete, nil
}
func (c *pcmConverter) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.resampler.close()
	c.resampler = nil
	c.pending = nil
}
