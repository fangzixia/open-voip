package middleware

import (
	"net/http"
	"strings"

	"open-call/internal/errs"
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
			next.ServeHTTP(w, r)
		})
	}
}
