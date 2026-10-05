package media

// snapshotSIPRTP 复制当前 SIP RTP 会话列表，避免在 RLock 下 range 时与其他协程写 map 冲突导致 panic。
func (r *room) snapshotSIPRTP() []*sipRTP {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	out := make([]*sipRTP, 0, len(r.sipRTP))
	for rt := range r.sipRTP {
		out = append(out, rt)
	}
	r.mu.RUnlock()
	return out
}
