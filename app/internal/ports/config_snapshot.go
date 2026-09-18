package ports

import (
	"context"
	"time"
)

// QueueSnapshot 队列只读快照，供 L3 路由与 IVR 使用。
type QueueSnapshot struct {
	// ID 队列 UUID。
	ID string
	// Name 队列名。
	Name string
	// VideoEnabled 是否视频队列。
	VideoEnabled bool
	// MaxWaitSec 最大等待秒数。
	MaxWaitSec int
	// IVRFlowID 绑定的 IVR 流程，可空。
	IVRFlowID string
}

// IVRNodeSnapshot IVR 节点简化表示（Phase 1 细化）。
type IVRNodeSnapshot struct {
	// Type 节点类型：play / menu / route_queue 等。
	Type string
	// Payload 节点配置 JSON。
	Payload string
}

// IVRSnapshot 已发布 IVR 快照。
type IVRSnapshot struct {
	// SnapshotID 快照 UUID。
	SnapshotID string
	// FlowID 流程 ID。
	FlowID string
	// Version 版本号。
	Version int
	// Root 根节点。
	Root IVRNodeSnapshot
}

// BusinessHours 工作时间窗口。
type BusinessHours struct {
	// Timezone IANA 时区名。
	Timezone string
	// WeekdayHours 周一到周日是否营业的简化 JSON 或结构，Phase 1 实现。
	WeekdayHours string
}

// ConfigSnapshotPort 由 L4 configpub 实现，L3 IVR/路由只读已发布配置。
type ConfigSnapshotPort interface {
	// GetQueue 读取队列快照；不存在返回 error。
	GetQueue(ctx context.Context, queueID string) (QueueSnapshot, error)
	// GetLatestIVR 读取流程最新 published 快照。
	GetLatestIVR(ctx context.Context, flowID string) (IVRSnapshot, error)
	// GetBusinessHours 读取全局或队列级工作时间。
	GetBusinessHours(ctx context.Context, queueID string) (BusinessHours, error)
	// Now 返回用于时间判断的「当前时间」（便于测试注入）。
	Now(ctx context.Context) time.Time
}
