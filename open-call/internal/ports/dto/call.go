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
	QueueID string `json:"queue_id"`
	// GuestSessionID 访客会话 ID，可空（Demo 直链）。
	GuestSessionID string `json:"guest_session_id"`
	// SessionType 期望媒介类型。
	SessionType SessionType `json:"session_type"`
	// Priority 入队优先级，0 为普通。
	Priority int `json:"priority"`
	// Caller 主叫标识（SIP FROM 或访客）。
	Caller string `json:"caller"`
	// SkipIVR 为 true 时即使队列绑定 IVR 也直接排队。
	SkipIVR bool `json:"skip_ivr"`
	// CallID 可选预分配通话 ID（SIP 呼入须先标记 PCMU 房间）。
	CallID string `json:"call_id"`
}

// OutboundRequest 坐席外呼或分机互拨请求。
type OutboundRequest struct {
	// AgentID 发起坐席。
	AgentID string `json:"agent_id"`
	// Destination 目标号码或分机。
	Destination string `json:"destination"`
	// TrunkID PSTN 中继 ID，可空（分机互拨不需要）。
	TrunkID string `json:"trunk_id"`
}

// TransferRequest 转接参数。
type TransferRequest struct {
	// Mode blind 或 consult。
	Mode string `json:"mode"`
	// TargetAgentID 目标坐席，与 TargetQueueID 二选一。
	TargetAgentID string `json:"target_agent_id"`
	// TargetQueueID 目标队列。
	TargetQueueID string `json:"target_queue_id"`
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
