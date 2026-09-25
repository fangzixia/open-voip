package control

import (
	"context"
	"hash/fnv"
	"open-switch/internal/observability"
	"open-switch/internal/ports"
)

type commandKey struct{}
type commandToken struct {
	service *Service
	index   uint32
}

// command 按通话串行执行命令；固定数量锁避免已结束通话无限积累锁对象。
// 同步内部调用沿用上下文，异步回调必须使用自己的上下文。
// command 按 callID 串行化命令；嵌套调用沿用上下文标记，避免再次加锁。
func (s *Service) command(ctx context.Context, callID string) (context.Context, func()) {
	ctx = observability.WithFields(ctx, observability.Fields{CallID: callID})
	h := fnv.New32a()
	_, _ = h.Write([]byte(callID))
	index := h.Sum32() % uint32(len(s.commands))
	if token, ok := ctx.Value(commandKey{}).(commandToken); ok && token.service == s && token.index == index {
		return ctx, func() {}
	}
	s.commands[index].Lock()
	return context.WithValue(ctx, commandKey{}, commandToken{s, index}), s.commands[index].Unlock
}

// ListCalls 返回当前活跃通话快照，供重连及实时报表查询。
func (s *Service) ListCalls(ctx context.Context) ([]ports.CallView, error) {
	s.mu.Lock()
	ids := make([]string, 0, len(s.calls))
	for id := range s.calls {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	out := make([]ports.CallView, 0, len(ids))
	for _, id := range ids {
		v, err := s.GetCall(ctx, id)
		if err != nil {
			return nil, err
		}
		if v.State != stateEnded {
			out = append(out, v)
		}
	}
	return out, nil
}
