package ports

import (
	"context"
	"time"
)

// SIPBinding 是本地注册服务器的一条 Contact 绑定。
type SIPBinding struct {
	AOR        string
	ContactURI string
	CallID     string
	CSeq       uint32
	// Addr 实际投递地址 host:port（已按 NAT 源地址修正）。
	Addr      string
	ExpiresAt time.Time
}

// SIPBindingStore 持久化注册绑定，使 Switch 重启后无需等话机重新注册即可呼叫。
type SIPBindingStore interface {
	LoadSIPBindings(ctx context.Context) ([]SIPBinding, error)
	// ReplaceSIPBindings 用给定集合整体替换某个 AOR 的绑定；空集合即删除。
	ReplaceSIPBindings(ctx context.Context, aor string, bindings []SIPBinding) error
}
