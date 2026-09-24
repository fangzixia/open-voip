package ports

import (
	"context"

	"open-call/internal/ports/dto"
)

// RecordingPolicyPort 由 L4 实现；L3 在通话进入媒体会话时查询，不得自行拼策略。
type RecordingPolicyPort interface {
	// ForQueue 返回队列录音策略。queueID 为空时返回组织默认策略。
	ForQueue(ctx context.Context, queueID string) (dto.RecordingPolicy, error)
}
