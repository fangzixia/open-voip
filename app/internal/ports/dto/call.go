// Package dto 存放跨层 Port 使用的数据传输对象，禁止引用 store 或各层实现包。
package dto

// SessionType 表示通话媒介类型。
type SessionType string

const (
	// SessionTypeAudio 纯语音会话。
	SessionTypeAudio SessionType = "audio"
	// SessionTypeVideo 纯视频会话（含音频）。
	SessionTypeVideo SessionType = "video"
	// SessionTypeMixed 语音为主、可含视频轨的混合会话。
	SessionTypeMixed SessionType = "mixed"
)

// InboundRequest 访客或呼入创建通话的请求参数。
type InboundRequest struct {
	// QueueID 目标队列 UUID。
	QueueID string
	// GuestSessionID 访客会话 ID，可空（Demo 直链）。
	GuestSessionID string
	// SessionType 期望媒介类型。
	SessionType SessionType
}

// OutboundRequest 坐席外呼或分机互拨请求。
type OutboundRequest struct {
	// AgentID 发起坐席。
	AgentID string
	// Destination 目标号码或分机。
	Destination string
}

// TransferRequest 转接参数。
type TransferRequest struct {
	// Mode blind 或 consult。
	Mode string
	// TargetAgentID 目标坐席，与 TargetQueueID 二选一。
	TargetAgentID string
	// TargetQueueID 目标队列。
	TargetQueueID string
}

// HangupReason 挂断原因枚举字符串。
type HangupReason string

const (
	HangupReasonNormal   HangupReason = "normal"
	HangupReasonTimeout  HangupReason = "timeout"
	HangupReasonAbandon  HangupReason = "abandon"
	HangupReasonError    HangupReason = "error"
	HangupReasonTransfer HangupReason = "transfer"
)
