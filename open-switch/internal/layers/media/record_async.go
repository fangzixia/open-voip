package media

import "sync"

// pcmMixAsync 将 flush/写盘操作放到单协程，避免在 RTP 读循环中阻塞。
type pcmMixAsync struct {
	mix *pcmMix
	ch  chan func()
	wg  sync.WaitGroup
}

func newPCMMixAsync(m *pcmMix) *pcmMixAsync {
	if m == nil {
		return nil
	}
	a := &pcmMixAsync{mix: m, ch: make(chan func(), 256)}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		for fn := range a.ch {
			if fn != nil {
				fn()
			}
		}
	}()
	return a
}

func (a *pcmMixAsync) close() {
	if a == nil {
		return
	}
	close(a.ch)
	a.wg.Wait()
}

func (a *pcmMixAsync) enqueue(fn func()) {
	if a == nil || fn == nil {
		return
	}
	select {
	case a.ch <- fn:
	default:
		fn()
	}
}
