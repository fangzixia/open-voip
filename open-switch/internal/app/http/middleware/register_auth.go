package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"open-switch/internal/config"
	"open-switch/internal/errs"
	"open-switch/internal/store"
)

// RegisterAuth 校验 integrator 登记请求的 register_token。
func RegisterAuth(cfg config.IntegrationConfig, registry *store.ApplicationRegistry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := strings.TrimSpace(cfg.RegisterToken)
			header := strings.TrimSpace(r.Header.Get("X-Register-Token"))
			if token != "" {
				if subtle.ConstantTimeCompare([]byte(header), []byte(token)) != 1 {
					writeAuthError(w, errs.Unauthorized("register token 无效"))
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if cfg.AllowOpenRegister && registry != nil && len(registry.List()) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			writeAuthError(w, errs.Forbidden("未配置 integration.register_token，拒绝登记"))
		})
	}
}
