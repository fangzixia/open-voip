// Package configpub 实现 L4 配置发布与 ConfigSnapshotPort 只读快照。
package configpub

import (
	"context"
	"time"

	"open-voip/internal/errs"
	"open-voip/internal/ports"
)

// SnapshotService 实现 ConfigSnapshotPort。
type SnapshotService struct{}

// NewSnapshotService 创建配置快照服务。
func NewSnapshotService() *SnapshotService {
	return &SnapshotService{}
}

var _ ports.ConfigSnapshotPort = (*SnapshotService)(nil)

func (s *SnapshotService) GetQueue(ctx context.Context, queueID string) (ports.QueueSnapshot, error) {
	return ports.QueueSnapshot{}, errs.ErrNotImplemented
}

func (s *SnapshotService) GetLatestIVR(ctx context.Context, flowID string) (ports.IVRSnapshot, error) {
	return ports.IVRSnapshot{}, errs.ErrNotImplemented
}

func (s *SnapshotService) GetBusinessHours(ctx context.Context, queueID string) (ports.BusinessHours, error) {
	return ports.BusinessHours{}, errs.ErrNotImplemented
}

func (s *SnapshotService) Now(ctx context.Context) time.Time {
	return time.Now().UTC()
}
