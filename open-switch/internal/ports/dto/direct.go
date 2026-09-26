package dto

// DirectCallRequest creates a call controlled by an external trusted service.
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
	Destination string `json:"destination"`
	TrunkID     string `json:"trunk_id,omitempty"`
}
