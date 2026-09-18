// Package cdr 实现 L4 话单写入与查询，供 CDRRecorderPort 使用。
package cdr

import (
	"context"

	"open-voip/internal/errs"
	"open-voip/internal/ports"
)

// RecorderService 实现 CDRRecorderPort。
type RecorderService struct{}

// NewRecorderService 创建话单服务。
func NewRecorderService() *RecorderService {
	return &RecorderService{}
}

var _ ports.CDRRecorderPort = (*RecorderService)(nil)

func (r *RecorderService) Upsert(ctx context.Context, req ports.CDRWriteRequest) error {
	return errs.ErrNotImplemented
}
