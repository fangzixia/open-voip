package middleware

import (
	"net/http"
	"strings"

	"open-call/internal/datetime"
	"open-call/internal/errs"
	"open-call/internal/layers/biz/auth"
)

// IntegrationAuth 校验 open-switch 回调使用的 integration secret。
func IntegrationAuth(secret string) func(http.Handler) http.Handler {
	sec := strings.TrimSpace(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if BearerToken(r) != sec {
				writeAuthError(w, errs.Unauthorized("integration 密钥无效"))
				return
			}
			if h := r.Header.Get("X-Principal"); h != "" {
				var p auth.Principal
				if err := datetime.Unmarshal([]byte(h), &p); err == nil {
					next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
