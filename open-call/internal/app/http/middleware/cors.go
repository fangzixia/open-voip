package middleware

import (
	"net/http"
	"net/url"
	"open-call/internal/httpapi"
	"strings"
)

const corsAllowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
const corsAllowHeaders = "Accept, Authorization, Content-Type, X-Request-ID, X-Trace-ID, X-Call-ID, X-Leg-ID, X-Client-Session-ID"

// CORS 仅允许同源请求或配置白名单中的浏览器来源。
func CORS(allowed []string) func(http.Handler) http.Handler {
	set := map[string]struct{}{}
	for _, v := range allowed {
		v = strings.TrimSuffix(strings.TrimSpace(v), "/")
		if v != "" {
			set[strings.ToLower(v)] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSuffix(strings.TrimSpace(r.Header.Get("Origin")), "/")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			u, err := url.Parse(origin)
			same := err == nil && strings.EqualFold(u.Host, r.Host)
			_, configured := set[strings.ToLower(origin)]
			if !same && !configured {
				httpapi.Failure(w, http.StatusForbidden, "forbidden", "浏览器来源不在允许列表")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
			w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
			w.Header().Set("Access-Control-Max-Age", "600")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-Trace-ID, Retry-After")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
