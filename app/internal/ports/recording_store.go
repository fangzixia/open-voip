package ports

import (
	"context"
	"time"
)

// RecordingMeta 录音元数据，由 L3 在 Start/StopRecording 时写入 L4。
type RecordingMeta struct {
	// ID 录音 UUID，与 MediaPort 返回值一致。
	ID string
	// CallID 通话 ID。
	CallID string
	// FilePath 落盘路径。
	FilePath string
	// MediaType audio / video_composite。
	MediaType string
	// StartedAt 开始时间。
	StartedAt time.Time
	// EndedAt 结束时间，未结束为空。
	EndedAt *time.Time
	// RetainUntil 保留截止。
	RetainUntil *time.Time
	// FileSize 字节数。
	FileSize int64
}

// RecordingStorePort 由 L4 实现，供 L3 持久化录音元数据。
type RecordingStorePort interface {
	// Save 插入或更新录音元数据。
	Save(ctx context.Context, rec RecordingMeta) error
}
