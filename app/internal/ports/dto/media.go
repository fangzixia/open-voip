package dto

import "time"

// LegRole 媒体腿角色。
type LegRole string

const (
	LegRoleCustomer   LegRole = "customer"
	LegRoleAgent      LegRole = "agent"
	LegRoleIVRBot     LegRole = "ivr_bot"
	LegRoleSupervisor LegRole = "supervisor"
	LegRolePSTN       LegRole = "pstn"
)

// RoomOptions 创建媒体 Room 时的选项。
type RoomOptions struct {
	// SessionType 初始会话类型。
	SessionType SessionType
	// EnableVideo 是否协商视频轨。
	EnableVideo bool
}

// LocalOffer WebRTC 本地 Offer 描述，供 HTTP 信令返回客户端。
type LocalOffer struct {
	// SDP Session Description Protocol 文本。
	SDP string
	// Type 固定 offer。
	Type string
}

// ICECandidateInit ICE candidate 初始化结构（与 WebRTC JSON 对齐的简化字段）。
type ICECandidateInit struct {
	// Candidate SDP 行。
	Candidate string
	// SDPMid 媒体标识。
	SDPMid string
	// SDPMLineIndex 行索引。
	SDPMLineIndex *uint16
}

// TURNConfig 下发给客户端的 TURN 配置。
type TURNConfig struct {
	// URLs TURN 服务器地址列表。
	URLs []string
	// Username 短期用户名。
	Username string
	// Credential 短期密码。
	Credential string
	// TTL 凭证有效时长。
	TTL time.Duration
}

// RecordingPolicy 单次录制策略快照。
type RecordingPolicy struct {
	// Mode off / audio / video_composite。
	Mode string
	// NotifyGuest 是否需对访客播放告知音。
	NotifyGuest bool
	// NotifyMessage 告知文案。
	NotifyMessage string
	// RetainDays 保留天数。
	RetainDays int
}

// AudioSource IVR 放音源描述。
type AudioSource struct {
	// FilePath WAV/MP3 路径。
	FilePath string
	// Loop 是否循环直到 Stop。
	Loop bool
}

// DTMFDigit DTMF 按键。
type DTMFDigit string
