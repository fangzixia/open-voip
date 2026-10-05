package middleware

import (
	"context"
	"net/http"
	"open-call/internal/httpapi"
	"open-call/internal/observability"
	"strings"

	"open-call/internal/errs"
	"open-call/internal/layers/biz/auth"
)

type authCtxKey int

const principalKey authCtxKey = 1

// Authenticator 解析 access / guest token。
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (auth.Principal, error)
}

// Auth 校验 Authorization Bearer 或忽略已公开路由。
func Auth(a Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := BearerToken(r)
			if token == "" {
				writeAuthError(w, errs.Unauthorized("未认证或令牌失效"))
				return
			}
			p, err := a.Authenticate(r.Context(), token)
			if err != nil {
				writeAuthError(w, err)
				return
			}
			if p.MustChangePassword && r.URL.Path != "/api/v1/auth/change-password" && r.URL.Path != "/api/v1/auth/logout" {
				writeAuthError(w, errs.Forbidden("必须先修改临时密码"))
				return
			}
			ctx := WithPrincipal(r.Context(), p)
			ctx = observability.With(ctx, observability.Context{AgentID: p.AgentID, CallID: p.GuestCallID})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// BearerToken 从 Header 提取 Bearer。
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// WithPrincipal 写入主体。
func WithPrincipal(ctx context.Context, p auth.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFromContext 读取主体。
func PrincipalFromContext(ctx context.Context) (auth.Principal, bool) {
	p, ok := ctx.Value(principalKey).(auth.Principal)
	return p, ok && (p.UserID != "" || p.GuestID != "")
}

// RequirePermission 校验当前角色分配产生的有效权限。
func RequirePermission(code string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFromContext(r.Context())
			if !ok {
				writeAuthError(w, errs.Unauthorized("未认证或令牌失效"))
				return
			}
			if !p.Has(code) {
				writeAuthError(w, errs.Forbidden("无权限"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeAuthError(w http.ResponseWriter, err error) { httpapi.Error(w, err) }
