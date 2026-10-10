package ports

import (
	"context"
	"time"
)

// RecordingMeta 录音元数据，由 L3 在 Start/StopRecording 时写入 L4。
type RecordingMeta struct {
	RecordingSemantics string `json:"recording_semantics"`
	Channels           int    `json:"channels"`
	DurationSamples    int64  `json:"duration_samples"`
	Status             string `json:"status"`
	FailureReason      string `json:"failure_reason,omitempty"`
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
	// SampleRateHz conversation_mono_v1 主录采样率为 8000 Hz。
	SampleRateHz int `json:"sample_rate_hz,omitempty"`
	// LegPaths 分轨 WAV：leg_id -> 文件路径。
	LegPaths map[string]string `json:"leg_paths,omitempty"`
}

// RecordingStorePort 由 L4 实现，供 L3 持久化录音元数据。
type RecordingStorePort interface {
	// Save 插入或更新录音元数据。
	Save(ctx context.Context, rec RecordingMeta) error
}
