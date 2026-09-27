package middleware

import (
	"crypto/subtle"
	"net/http"
	"open-switch/internal/config"
	"open-switch/internal/httpapi"
	"open-switch/internal/scope"
	"strings"

	"open-switch/internal/errs"
)

// ApplicationAuth authenticates a trusted business service and binds its scope.
func ApplicationAuth(apps []config.ApplicationConfig) func(http.Handler) http.Handler {
	bySecret := make(map[string]string, len(apps))
	for _, app := range apps {
		bySecret[strings.TrimSpace(app.Secret)] = strings.TrimSpace(app.ID)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			applicationID := ""
			for secret, id := range bySecret {
				if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1 {
					applicationID = id
				}
			}
			if applicationID == "" {
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
