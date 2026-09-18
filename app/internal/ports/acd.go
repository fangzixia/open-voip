package ports

import (
	"context"

	"open-voip/internal/ports/dto"
)

// ACDDispatchPort 由 L4 queue/acd 实现，L3 在需要分配坐席时调用；L4 不得 import Call FSM。
type ACDDispatchPort interface {
	// RequestAgent 根据队列策略与技能返回一名可用坐席；无可用坐席时 AgentID 为空且不报错。
	RequestAgent(ctx context.Context, req dto.DispatchRequest) (dto.DispatchResult, error)
}
