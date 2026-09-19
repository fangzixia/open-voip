package models

import "time"

// CDR 话单记录，由 L3 经 CDRRecorderPort 写入。
type CDR struct {
	// ID 话单 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:话单 ID"`
	// CallID 关联通话。
	CallID string `gorm:"type:uuid;uniqueIndex;not null;comment:通话 ID"`
	// Direction inbound / outbound / internal。
	Direction string `gorm:"size:16;comment:呼叫方向"`
	// QueueID 队列 ID，可空。
	QueueID *string `gorm:"type:uuid;index:idx_cdr_started_queue,priority:2;comment:队列 ID"`
	// AgentID 接听坐席，可空。
	AgentID *string `gorm:"type:uuid;index;comment:坐席 ID"`
	// Caller 主叫标识（分机或访客）。
	Caller string `gorm:"size:64;comment:主叫"`
	// Callee 被叫标识。
	Callee string `gorm:"size:64;comment:被叫"`
	// SessionType audio / video / mixed。
	SessionType string `gorm:"size:16;not null;comment:媒介类型"`
	// Result 结果：answered / abandoned / failed 等。
	Result string `gorm:"size:32;not null;index;comment:通话结果"`
	// StartedAt 起始时间。
	StartedAt time.Time `gorm:"index:idx_cdr_started_queue,priority:1;comment:开始时间"`
	// AnsweredAt 接通时间，可空。
	AnsweredAt *time.Time `gorm:"comment:接通时间"`
	// EndedAt 结束时间。
	EndedAt *time.Time `gorm:"comment:结束时间"`
	// DurationSec 计费时长秒数。
	DurationSec int `gorm:"not null;default:0;comment:通话时长秒"`
	// WaitSec 排队等待秒数。
	WaitSec int `gorm:"not null;default:0;comment:等待秒数"`
	// VideoStartedAt 视频轨开始时间，可空。
	VideoStartedAt *time.Time `gorm:"comment:视频开始时间"`
	// VideoUpgradeOk 语音升视频是否成功。
	VideoUpgradeOk bool `gorm:"not null;default:false;comment:升视频是否成功"`
	// ScreenShareCount 屏幕共享次数。
	ScreenShareCount int `gorm:"not null;default:0;comment:屏幕共享次数"`
	// CreatedAt 写入时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (CDR) TableName() string { return "cdr" }
