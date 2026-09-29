package middleware

import (
	"net/http"
	"strings"

	"open-switch/internal/errs"
	"open-switch/internal/httpapi"
	"open-switch/internal/scope"
	"open-switch/internal/store"
)

// ApplicationAuth 校验受信任业务系统凭据，并将应用 ID 写入请求上下文。
func ApplicationAuth(registry *store.ApplicationRegistry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			applicationID, ok := registry.ResolveSecret(token)
			if !ok {
				writeAuthError(w, errs.Unauthorized("业务系统凭据无效"))
				return
			}
			next.ServeHTTP(w, r.WithContext(scope.WithApplication(r.Context(), applicationID)))
		})
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func writeAuthError(w http.ResponseWriter, err error) { httpapi.Error(w, err) }
