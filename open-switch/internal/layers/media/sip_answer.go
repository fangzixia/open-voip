// 本文件负责呼入 SIP 腿的应答时序：200 OK 发出前登记的回调延后到应答后执行。
package media

import "sync"

type answerGate struct {
	mu      sync.Mutex
	pending map[string][]func()
}

func (g *answerGate) markPending(callID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending == nil {
		g.pending = map[string][]func(){}
	}
	if _, ok := g.pending[callID]; !ok {
		g.pending[callID] = nil
	}
}

// release 取出待执行回调并清除待应答标记；answered=false 时丢弃回调（呼叫已失败）。
func (g *answerGate) release(callID string, answered bool) []func() {
	g.mu.Lock()
	defer g.mu.Unlock()
	fns, ok := g.pending[callID]
	if !ok {
		return nil
	}
	delete(g.pending, callID)
	if !answered {
		return nil
	}
	return fns
}

// DeferUntilAnswered 若呼入 SIP 腿尚未发出 200 OK，则登记 fn 在应答后异步执行并返回 true；
// 否则返回 false 且不调用 fn，由调用方直接继续。
func (s *Service) DeferUntilAnswered(callID string, fn func()) bool {
	s.answers.mu.Lock()
	defer s.answers.mu.Unlock()
	if _, ok := s.answers.pending[callID]; !ok {
		return false
	}
	s.answers.pending[callID] = append(s.answers.pending[callID], fn)
	return true
}

func (s *Service) markSIPAnswerPending(callID string) { s.answers.markPending(callID) }

func (s *Service) finishSIPAnswer(callID string, answered bool) {
	for _, fn := range s.answers.release(callID, answered) {
		go fn()
	}
}
