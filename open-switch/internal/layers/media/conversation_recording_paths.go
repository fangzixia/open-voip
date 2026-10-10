package media

import (
	"strings"
)

// sanitizeLegID 将 leg UUID 转为安全文件名片段。
func sanitizeLegID(legID string) string {
	legID = strings.TrimSpace(legID)
	if legID == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range legID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "leg"
	}
	if len(out) > 48 {
		return out[:48]
	}
	return out
}
