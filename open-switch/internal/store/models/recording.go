package models

import "time"

// Recording 录音/录像文件元数据。
type Recording struct {
	RecordingSemantics string `gorm:"size:64;not null;default:legacy"`
	Channels           int
	DurationSamples    int64
	Status             string `gorm:"size:32;not null;default:completed"`
	FailureReason      string `gorm:"type:text"`
	SampleRateHz       int
	LegPaths           map[string]string `gorm:"serializer:json;type:jsonb"`
	// ID 录制 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:录音记录 ID"`
	// CallID 关联通话。
	CallID string `gorm:"type:uuid;index;not null;comment:通话 ID"`
	// FilePath 磁盘相对或绝对路径。
	FilePath string `gorm:"size:512;not null;comment:文件路径"`
	// MediaType 媒体类型：audio / video_composite。
	MediaType string `gorm:"size:32;not null;comment:录制类型"`
	// StartedAt 开始录制时间。
	StartedAt time.Time `gorm:"comment:开始时间"`
	// EndedAt 结束时间。
	EndedAt *time.Time `gorm:"comment:结束时间"`
	// RetainUntil 保留截止时间；Switch 不写入，由业务侧自行管理生命周期。
	RetainUntil *time.Time `gorm:"index;comment:保留截止时间"`
	// FileSize 字节数。
	FileSize int64 `gorm:"comment:文件大小字节"`
	// CreatedAt 元数据写入时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (Recording) TableName() string { return "os_recordings" }
