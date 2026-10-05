package middleware

import (
	"net/http"
	"net/url"
	"open-call/internal/httpapi"
	"strings"
)

const corsAllowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
const corsAllowHeaders = "Accept, Authorization, Content-Type, X-Request-ID, X-Trace-ID, X-Call-ID, X-Leg-ID, X-Client-Session-ID"

func originsAllowAll(allowed []string) bool {
	for _, v := range allowed {
		if strings.TrimSpace(v) == "*" {
			return true
		}
	}
	return false
}

func originPermitted(origin, host string, allowAll bool, allowed map[string]struct{}) bool {
	if origin == "" {
		return true
	}
	if allowAll {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Host, host) {
		return true
	}
	_, configured := allowed[strings.ToLower(origin)]
	return configured
}

func setCORSHeaders(w http.ResponseWriter, origin string) {
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
	w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
	w.Header().Set("Access-Control-Max-Age", "600")
	w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-Trace-ID, Retry-After")
}

// CORS 校验浏览器 Origin；allowed_origins 含 "*" 时不限制来源（仍回显请求的 Origin）。
func CORS(allowed []string) func(http.Handler) http.Handler {
	set := map[string]struct{}{}
	for _, v := range allowed {
		v = strings.TrimSuffix(strings.TrimSpace(v), "/")
		if v != "" && v != "*" {
			set[strings.ToLower(v)] = struct{}{}
		}
	}
	allowAll := originsAllowAll(allowed)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSuffix(strings.TrimSpace(r.Header.Get("Origin")), "/")
			if !originPermitted(origin, r.Host, allowAll, set) {
				httpapi.Failure(w, http.StatusForbidden, "forbidden", "浏览器来源不在允许列表")
				return
			}
			setCORSHeaders(w, origin)
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// OriginsAllowAll 表示配置是否包含通配来源 "*"。
func OriginsAllowAll(allowed []string) bool { return originsAllowAll(allowed) }
