package middleware

import (
	"context"
	"net/http"

	"open-switch/internal/authctx"
	"open-switch/internal/errs"
	"open-switch/internal/httpapi"
)

type authCtxKey int

const principalKey authCtxKey = 1

// WithPrincipal 写入主体。
func WithPrincipal(ctx context.Context, p authctx.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func RequireCapability(code string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFromContext(r.Context())
			if !ok {
				httpapi.Error(w, errs.Unauthorized("未认证"))
				return
			}
			if !p.Has(code) && !(p.IsGuest() && (code == "calls.read" || code == "calls.operate")) {
				httpapi.Error(w, errs.Forbidden("无权限"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PrincipalFromContext 读取主体。
func PrincipalFromContext(ctx context.Context) (authctx.Principal, bool) {
	p, ok := ctx.Value(principalKey).(authctx.Principal)
	return p, ok && (p.UserID != "" || p.GuestID != "" || p.AgentID != "")
}
