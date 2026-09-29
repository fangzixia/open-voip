package dto

// DirectCallRequest 由外部受信任服务发起的直控通话创建参数。
type DirectCallRequest struct {
	CallID         string      `json:"call_id"`
	Direction      string      `json:"direction"`
	Caller         string      `json:"caller"`
	Callee         string      `json:"callee"`
	SessionType    SessionType `json:"session_type"`
	InitialLegRole LegRole     `json:"initial_leg_role"`
	AgentID        string      `json:"agent_id,omitempty"`
}

type DirectLegRequest struct {
	Role    LegRole `json:"role"`
	AgentID string  `json:"agent_id,omitempty"`
}

type DirectSIPRequest struct {
	Destination  string `json:"destination"`
	TrunkID      string `json:"trunk_id,omitempty"`
	RouteGroupID string `json:"route_group_id,omitempty"`
}

// StubCallRequest 创建无媒体腿的空 Call，供后续加腿（方案 POST /calls）。
type StubCallRequest struct {
	CallID      string         `json:"call_id"`
	BusinessRef string         `json:"business_ref,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}
