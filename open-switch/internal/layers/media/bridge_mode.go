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
	if r == nil || legA == "" || legB == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.bridgeAllowed() {
		return
	}
	r.direct = true
	r.bridgeA, r.bridgeB = legA, legB
}
