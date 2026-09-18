package queue

import (
	"context"

	"open-voip/internal/errs"
	"open-voip/internal/ports"
	"open-voip/internal/ports/dto"
)

// RecordingPolicyService 实现 RecordingPolicyPort。
type RecordingPolicyService struct{}

// NewRecordingPolicyService 创建录音策略服务。
func NewRecordingPolicyService() *RecordingPolicyService {
	return &RecordingPolicyService{}
}

var _ ports.RecordingPolicyPort = (*RecordingPolicyService)(nil)

func (r *RecordingPolicyService) ForQueue(ctx context.Context, queueID string) (dto.RecordingPolicy, error) {
	return dto.RecordingPolicy{}, errs.ErrNotImplemented
}
