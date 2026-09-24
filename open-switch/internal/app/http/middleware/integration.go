package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"open-switch/internal/authctx"
	"open-switch/internal/errs"
)

// IntegrationAuth 校验服务间 integration secret，并可选解析 X-Principal。
func IntegrationAuth(secret string) func(http.Handler) http.Handler {
	sec := strings.TrimSpace(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if bearerToken(r) != sec {
				writeAuthError(w, errs.Unauthorized("integration 密钥无效"))
				return
			}
			ctx := r.Context()
			if h := r.Header.Get("X-Principal"); h != "" {
				var p authctx.Principal
				if err := json.Unmarshal([]byte(h), &p); err == nil {
					ctx = WithPrincipal(ctx, p)
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
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

func writeAuthError(w http.ResponseWriter, err error) {
	api := errs.AsAPIError(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(api.HTTP)
	msg := api.Message
	if msg == "" {
		msg = api.Kind
	}
	body := `{"error":"` + api.Kind + `","message":"` + jsonEscape(msg) + `"`
	if api.Code != "" {
		body += `,"code":"` + api.Code + `"`
	}
	body += "}"
	_, _ = w.Write([]byte(body))
}

func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
