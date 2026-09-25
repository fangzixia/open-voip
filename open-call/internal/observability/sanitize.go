package observability

import (
	"fmt"
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

var (
	jwtPattern    = regexp.MustCompile(`(?i)\beyJ[a-zA-Z0-9_-]{8,}\.[a-zA-Z0-9_-]{8,}(?:\.[a-zA-Z0-9_-]*)?`)
	bearerPattern = regexp.MustCompile(`(?i)\bBearer\s+[^\s,;]+`)
	icePwdPattern = regexp.MustCompile(`(?im)(a=ice-pwd:)[^\r\n]+`)
	headerPattern = regexp.MustCompile(`(?im)\b(authorization|cookie|set-cookie)\s*:\s*[^\r\n]+`)
	secretPattern = regexp.MustCompile(`(?i)\b(jwt|token|password|credential|ice-pwd)\s*[:=]\s*[^\s,;&\r\n]+`)
)

// SanitizeMap recursively copies fields while removing credentials and SDP ICE passwords.
func SanitizeMap(fields map[string]any) map[string]any {
	if fields == nil {
		return nil
	}
	out := make(map[string]any, len(fields))
	for key, value := range fields {
		if sensitiveKey(key) {
			out[key] = redacted
			continue
		}
		out[key] = sanitizeValue(value)
	}
	return out
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case string:
		return SanitizeString(typed)
	case map[string]any:
		return SanitizeMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = sanitizeValue(typed[i])
		}
		return out
	case []string:
		out := make([]string, len(typed))
		for i := range typed {
			out[i] = SanitizeString(typed[i])
		}
		return out
	default:
		return value
	}
}

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(key))
	if normalized == "authorization" || normalized == "cookie" || normalized == "set_cookie" ||
		normalized == "jwt" || normalized == "password" || normalized == "credential" ||
		normalized == "credentials" || normalized == "ice_pwd" || normalized == "ice_password" {
		return true
	}
	return normalized == "token" || strings.HasSuffix(normalized, "_token") ||
		strings.Contains(normalized, "password") || strings.Contains(normalized, "credential")
}

// SanitizeString redacts bearer/JWT material and SDP a=ice-pwd lines.
func SanitizeString(value string) string {
	value = headerPattern.ReplaceAllString(value, "${1}: "+redacted)
	value = bearerPattern.ReplaceAllString(value, "Bearer "+redacted)
	value = jwtPattern.ReplaceAllString(value, redacted)
	value = secretPattern.ReplaceAllString(value, "${1}="+redacted)
	return icePwdPattern.ReplaceAllString(value, fmt.Sprintf("${1}%s", redacted))
}
