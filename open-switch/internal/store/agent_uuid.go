package store

func agentUUIDPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func agentUUIDStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
