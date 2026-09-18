// Package queue 实现 L4 队列配置与 ACD 选人算法。
package queue

import (
	"context"

	"open-voip/internal/errs"
	"open-voip/internal/ports"
	"open-voip/internal/ports/dto"
)

// ACDService 实现 ACDDispatchPort。
type ACDService struct{}

// NewACDService 创建 ACD 服务。
func NewACDService() *ACDService {
	return &ACDService{}
}

var _ ports.ACDDispatchPort = (*ACDService)(nil)

// RequestAgent Phase 0 占位，Phase 1 实现 longest_idle / round_robin。
func (a *ACDService) RequestAgent(ctx context.Context, req dto.DispatchRequest) (dto.DispatchResult, error) {
	return dto.DispatchResult{}, errs.ErrNotImplemented
}
