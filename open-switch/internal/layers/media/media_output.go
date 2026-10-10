package media

import "sync"

// One bounded sender per destination. A slow socket cannot run on the room
// clock. Overflow discards expired audio rather than accumulating latency.
type mediaOutput struct {
	mu      sync.Mutex
	queue   chan []byte
	done    chan struct{}
	once    sync.Once
	dropped uint64
}

func newMediaOutput(write func([]byte)) *mediaOutput {
	o := &mediaOutput{queue: make(chan []byte, 10), done: make(chan struct{})}
	go func() {
		for {
			select {
			case <-o.done:
				return
			case raw := <-o.queue:
				write(raw)
			}
		}
	}()
	return o
}
func (o *mediaOutput) stop() { o.once.Do(func() { close(o.done) }) }
func (o *mediaOutput) enqueue(raw []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	select {
	case <-o.done:
		return
	default:
	}
	select {
	case o.queue <- raw:
		return
	default:
	}
	select {
	case <-o.queue:
		o.dropped++
	default:
	}
	select {
	case o.queue <- raw:
	default:
		o.dropped++
	}
}
func (m *scheduledRoomMixer) send(id string, raw []byte, write func([]byte)) {
	m.mu.Lock()
	select {
	case <-m.stopCh:
		m.mu.Unlock()
		return
	default:
	}
	o := m.outputs[id]
	if o == nil {
		o = newMediaOutput(write)
		m.outputs[id] = o
	}
	m.mu.Unlock()
	o.enqueue(raw)
}
