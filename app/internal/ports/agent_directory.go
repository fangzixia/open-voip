package ports

import "context"

// AgentInfo 坐席目录条目，供 L3 振铃与视频能力判断。
type AgentInfo struct {
	// AgentID 坐席 UUID。
	AgentID string
	// UserID 关联用户 ID。
	UserID string
	// Extension 分机号。
	Extension string
	// VideoCapable 是否支持视频。
	VideoCapable bool
	// DisplayName 展示名。
	DisplayName string
}

// AgentDirectoryPort 由 L4 agent 包实现，L3 解析分机与能力。
type AgentDirectoryPort interface {
	// ByExtension 根据分机号查询坐席。
	ByExtension(ctx context.Context, extension string) (AgentInfo, error)
	// ByID 根据坐席 ID 查询。
	ByID(ctx context.Context, agentID string) (AgentInfo, error)
}
