// Package agent 实现 L4 坐席组织、签入状态与坐席目录 Port。
package agent

import (
	"context"

	"open-voip/internal/errs"
	"open-voip/internal/ports"
)

// DirectoryService 实现 AgentDirectoryPort。
type DirectoryService struct{}

// NewDirectoryService 创建坐席目录服务。
func NewDirectoryService() *DirectoryService {
	return &DirectoryService{}
}

var _ ports.AgentDirectoryPort = (*DirectoryService)(nil)

func (d *DirectoryService) ByExtension(ctx context.Context, extension string) (ports.AgentInfo, error) {
	return ports.AgentInfo{}, errs.ErrNotImplemented
}

func (d *DirectoryService) ByID(ctx context.Context, agentID string) (ports.AgentInfo, error) {
	return ports.AgentInfo{}, errs.ErrNotImplemented
}
