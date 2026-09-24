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
	SessionType SessionType `json:"session_type"`
	// EnableVideo 是否协商视频轨。
	EnableVideo bool `json:"enable_video"`
}

// LocalOffer WebRTC 本地 Offer 描述，供 HTTP 信令返回客户端。
type LocalOffer struct {
	// SDP Session Description Protocol 文本。
	SDP string `json:"sdp"`
	// Type 固定 offer。
	Type string `json:"type"`
}

// ICECandidateInit ICE candidate 初始化结构（与 WebRTC JSON 对齐的简化字段）。
type ICECandidateInit struct {
	// Candidate SDP 行。
	Candidate string `json:"candidate"`
	// SDPMid 媒体标识。
	SDPMid string `json:"sdp_mid"`
	// SDPMLineIndex 行索引。
	SDPMLineIndex *uint16 `json:"sdpm_line_index"`
}

// TURNConfig 下发给客户端的 TURN 配置。
type TURNConfig struct {
	STUNURLs []string `json:"stun_urls,omitempty"`
	// URLs TURN 服务器地址列表。
	URLs []string `json:"ur_ls"`
	// Username 短期用户名。
	Username string `json:"username"`
	// Credential 短期密码。
	Credential string `json:"credential"`
	// TTL 凭证有效时长。
	TTL time.Duration `json:"ttl"`
}

// RecordingPolicy 单次录制策略快照。
type RecordingPolicy struct {
	// Mode 录制模式：off / audio / video_composite。
	Mode string `json:"mode"`
	// NotifyGuest 是否需对访客播放告知音。
	NotifyGuest bool `json:"notify_guest"`
	// NotifyMessage 告知文案。
	NotifyMessage string `json:"notify_message"`
	// RetainDays 保留天数。
	RetainDays int `json:"retain_days"`
}

// AudioSource IVR 放音源描述。
type AudioSource struct {
	// FilePath WAV/MP3 路径。
	FilePath string `json:"file_path"`
	// Loop 是否循环直到 Stop。
	Loop bool `json:"loop"`
}

// DTMFDigit DTMF 按键。
type DTMFDigit string
