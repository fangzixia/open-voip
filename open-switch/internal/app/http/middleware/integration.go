package middleware

import (
	"net/http"
	"open-switch/internal/httpapi"
	"strings"

	"open-switch/internal/errs"
)

// IntegrationAuth authenticates a trusted service; end-user identity stays upstream.
func IntegrationAuth(secret string) func(http.Handler) http.Handler {
	sec := strings.TrimSpace(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if bearerToken(r) != sec {
				writeAuthError(w, errs.Unauthorized("integration 密钥无效"))
				return
			}
			next.ServeHTTP(w, r)
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
