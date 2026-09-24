package middleware

import (
	"context"

	"open-switch/internal/authctx"
)

type authCtxKey int

const principalKey authCtxKey = 1

// WithPrincipal 写入主体。
func WithPrincipal(ctx context.Context, p authctx.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFromContext 读取主体。
func PrincipalFromContext(ctx context.Context) (authctx.Principal, bool) {
	p, ok := ctx.Value(principalKey).(authctx.Principal)
	return p, ok && (p.UserID != "" || p.GuestID != "" || p.AgentID != "")
}
