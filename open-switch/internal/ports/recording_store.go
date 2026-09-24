package ports

import (
	"context"
	"time"
)

// RecordingMeta 录音元数据，由 L3 在 Start/StopRecording 时写入 L4。
type RecordingMeta struct {
	// ID 录音 UUID，与 MediaPort 返回值一致。
	ID string `json:"id"`
	// CallID 通话 ID。
	CallID string `json:"call_id"`
	// FilePath 落盘路径。
	FilePath string `json:"file_path"`
	// MediaType 媒体类型：audio / video_composite。
	MediaType string `json:"media_type"`
	// StartedAt 开始时间。
	StartedAt time.Time `json:"started_at"`
	// EndedAt 结束时间，未结束为空。
	EndedAt *time.Time `json:"ended_at"`
	// RetainUntil 保留截止。
	RetainUntil *time.Time `json:"retain_until"`
	// FileSize 字节数。
	FileSize int64 `json:"file_size"`
}

// RecordingStorePort 由 L4 实现，供 L3 持久化录音元数据。
type RecordingStorePort interface {
	// Save 插入或更新录音元数据。
	Save(ctx context.Context, rec RecordingMeta) error
}
