package models

import "time"

// Recording 录音/录像文件元数据。
type Recording struct {
	// ID 录制 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:录音记录 ID"`
	// CallID 关联通话。
	CallID string `gorm:"type:uuid;index;not null;comment:通话 ID"`
	// FilePath 磁盘相对或绝对路径。
	FilePath string `gorm:"size:512;not null;comment:文件路径"`
	// MediaType audio / video_composite。
	MediaType string `gorm:"size:32;not null;comment:录制类型"`
	// StartedAt 开始录制时间。
	StartedAt time.Time `gorm:"comment:开始时间"`
	// EndedAt 结束时间。
	EndedAt *time.Time `gorm:"comment:结束时间"`
	// CreatedAt 元数据写入时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (Recording) TableName() string { return "recordings" }

// QAMark 质检时间戳标记。
type QAMark struct {
	// ID 标记 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:质检标记 ID"`
	// CallID 通话 ID。
	CallID string `gorm:"type:uuid;index;not null;comment:通话 ID"`
	// OffsetSec 相对录音起点秒数。
	OffsetSec int `gorm:"not null;comment:相对秒数"`
	// Label 标记说明。
	Label string `gorm:"size:256;comment:标记说明"`
	// CreatedBy 操作人 user ID。
	CreatedBy string `gorm:"type:uuid;comment:创建人"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (QAMark) TableName() string { return "qa_marks" }

// CallWrapUp 通话小结文本。
type CallWrapUp struct {
	// ID 小结 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:小结 ID"`
	// CallID 通话 ID。
	CallID string `gorm:"type:uuid;uniqueIndex;not null;comment:通话 ID"`
	// AgentID 提交坐席。
	AgentID string `gorm:"type:uuid;index;not null;comment:坐席 ID"`
	// Notes 小结内容。
	Notes string `gorm:"type:text;not null;comment:小结文本"`
	// CreatedAt 提交时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (CallWrapUp) TableName() string { return "call_wrap_ups" }
