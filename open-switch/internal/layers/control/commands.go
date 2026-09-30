package control

import (
	"context"
	"hash/fnv"
	"open-switch/internal/observability"
	"open-switch/internal/ports"
	"open-switch/internal/scope"
)

type commandKey struct{}
type commandToken struct {
	service *Service
	callID  string
	index   uint32
}

// command 按 callID 串行化命令；嵌套调用仅当同一 callID 时跳过加锁（避免桶碰撞误放行）。
func (s *Service) command(ctx context.Context, callID string) (context.Context, func()) {
	ctx = observability.WithFields(ctx, observability.Fields{CallID: callID})
	h := fnv.New32a()
	_, _ = h.Write([]byte(callID))
	index := h.Sum32() % uint32(len(s.commands))
	if token, ok := ctx.Value(commandKey{}).(commandToken); ok && token.service == s && token.callID == callID {
		return ctx, func() {}
	}
	s.commands[index].Lock()
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt != nil && rt.rec.ConfigVersion != nil {
		ctx = scope.WithConfigVersion(ctx, *rt.rec.ConfigVersion)
	}
	return context.WithValue(ctx, commandKey{}, commandToken{s, callID, index}), s.commands[index].Unlock
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
