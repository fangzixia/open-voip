package media

import "time"

// bridgeAllowed 无活跃 IVR/等待音淡出时方可进入桥接模式。
func (r *room) bridgeAllowed() bool {
	if r == nil {
		return false
	}
	if !r.promptStopAt.IsZero() && time.Now().Before(r.promptStopAt) {
		return false
	}
	return true
}

func (s *Service) enterBridgeMode(r *room, legA, legB string) {
	if r == nil || legA == "" || legB == "" || !r.bridgeAllowed() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.mixer != nil {
		r.mixer.stop()
		r.mixer = nil
	}
	r.direct = true
	r.mixAudio = false
	r.bridgeA, r.bridgeB = legA, legB
}
