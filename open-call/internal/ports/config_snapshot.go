package ports

import (
	"context"
	"time"
)

// QueueSnapshot 队列只读快照，供 L3 路由与 IVR 使用。
type QueueSnapshot struct {
	// ID 队列 UUID。
	ID string `json:"id"`
	// Name 队列名。
	Name string `json:"name"`
	// VideoEnabled 是否视频队列。
	VideoEnabled bool `json:"video_enabled"`
	// MaxWaitSec 最大等待秒数。
	MaxWaitSec int `json:"max_wait_sec"`
	// IVRFlowID 绑定的 IVR 流程，可空。
	IVRFlowID string `json:"ivr_flow_id"`
	// OverflowAction 溢出动作：hangup / voicemail / queue。
	OverflowAction string `json:"overflow_action"`
	// OverflowQueueID 溢出目标队列。
	OverflowQueueID string `json:"overflow_queue_id"`
	// WaitPrompt 排队文案。
	WaitPrompt string `json:"wait_prompt"`
	// AnnounceRecording 是否告知录音。
	AnnounceRecording bool `json:"announce_recording"`
	// SkillIDs 所需技能。
	SkillIDs []string `json:"skill_i_ds"`
	// AfterHoursAction 非工作时间动作。
	AfterHoursAction string `json:"after_hours_action"`
	// ForceHangupOnCheckout 强制签出是否挂断。
	ForceHangupOnCheckout bool `json:"force_hangup_on_checkout"`
	// ListenAnnounce 班长监听时是否向客户播放提示。
	ListenAnnounce bool `json:"listen_announce"`
	// PriorityEnabled 是否允许 VIP/高优先级入队。
	PriorityEnabled bool `json:"priority_enabled"`
}

// IVRNodeSnapshot IVR 节点简化表示（Phase 1 细化）。
type IVRNodeSnapshot struct {
	// Type 节点类型：play / menu / route_queue 等。
	Type string `json:"type"`
	// Payload 节点配置 JSON。
	Payload string `json:"payload"`
}

// IVRSnapshot 已发布 IVR 快照。
type IVRSnapshot struct {
	// SnapshotID 快照 UUID。
	SnapshotID string `json:"snapshot_id"`
	// FlowID 流程 ID。
	FlowID string `json:"flow_id"`
	// Version 版本号。
	Version int `json:"version"`
	// PayloadJSON 节点树 JSON。
	PayloadJSON string `json:"payload_json"`
	// Root 兼容旧字段的根节点摘要。
	Root IVRNodeSnapshot `json:"root"`
}

// BusinessHours 工作时间窗口。
type BusinessHours struct {
	// Timezone IANA 时区名。
	Timezone string `json:"timezone"`
	// WeekdayHours 周一到周日是否营业的简化 JSON 或结构，Phase 1 实现。
	WeekdayHours string `json:"weekday_hours"`
}

// ConfigSnapshotPort 由 L4 configpub 实现，L3 IVR/路由只读已发布配置。
type ConfigSnapshotPort interface {
	// GetQueue 读取队列快照；不存在返回 error。
	GetQueue(ctx context.Context, queueID string) (QueueSnapshot, error)
	// GetLatestIVR 读取流程最新 published 快照。
	GetLatestIVR(ctx context.Context, flowID string) (IVRSnapshot, error)
	// GetBusinessHours 读取全局或队列级工作时间。
	GetBusinessHours(ctx context.Context, queueID string) (BusinessHours, error)
	// ResolveDID 将 DID/外显号码解析为队列 ID。
	ResolveDID(ctx context.Context, did string) (queueID string, err error)
	// Now 返回用于时间判断的「当前时间」（便于测试注入）。
	Now(ctx context.Context) time.Time
}
