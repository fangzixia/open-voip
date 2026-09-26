// Package authctx 定义 HTTP 集成鉴权传递的主体（无 JWT/DB 依赖）。
package authctx

// Principal 已认证主体，由 open-call 经 X-Principal 传入。
type Principal struct {
	UserID      string   `json:"user_id,omitempty"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions,omitempty"`
	AgentID     string   `json:"agent_id,omitempty"`
	GuestID     string   `json:"guest_id,omitempty"`
	GuestCallID string   `json:"guest_call_id,omitempty"`
}

// IsGuest 是否访客。
func (p Principal) IsGuest() bool {
	return p.Role == "guest"
}
func (p Principal) Has(code string) bool {
	for _, v := range p.Permissions {
		if v == code {
			return true
		}
	}
	return false
}
