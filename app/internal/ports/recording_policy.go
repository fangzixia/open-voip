package ports

import (
	"context"

	"open-voip/internal/ports/dto"
)

// RecordingPolicyPort 由 L4 queue 实现，L3 在 Answer 后查询录音策略。
type RecordingPolicyPort interface {
	// ForQueue 返回队列级录音策略。
	ForQueue(ctx context.Context, queueID string) (dto.RecordingPolicy, error)
}
