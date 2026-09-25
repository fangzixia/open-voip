package middleware

import (
	"net"
	"net/http"
	"open-call/internal/httpapi"
	"sync"
	"time"
)

type rateEntry struct {
	window time.Time
	count  int
}

// RateLimit 创建按来源 IP 统计的每分钟固定窗口限流中间件。
// 多实例生产部署还应在入口代理配置相同或更严格的集群级限流。
func RateLimit(limit int) func(http.Handler) http.Handler {
	if limit <= 0 {
		limit = 1
	}
	var mu sync.Mutex
	entries := map[string]rateEntry{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := time.Now().UTC()
			window := now.Truncate(time.Minute)
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			mu.Lock()
			entry := entries[host]
			if entry.window != window {
				entry = rateEntry{window: window}
			}
			entry.count++
			entries[host] = entry
			if len(entries) > 4096 {
				for key, item := range entries {
					if item.window.Before(window) {
						delete(entries, key)
					}
				}
			}
			allowed := entry.count <= limit
			mu.Unlock()
			if !allowed {
				w.Header().Set("Retry-After", "60")
				httpapi.Failure(w, http.StatusTooManyRequests, "rate_limited", "请求过于频繁")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
