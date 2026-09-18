package middleware

import (
	"context"
	"net/http"
)

type authCtxKey int

const (
	userIDKey authCtxKey = iota
	userRoleKey
)

// Auth JWT 鉴权占位：Phase 1 实现完整校验；Phase 0 仅透传并预留 context 键。
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// WithUser 将已认证用户写入 context（供 Phase 1 使用）。
func WithUser(ctx context.Context, userID, role string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	return context.WithValue(ctx, userRoleKey, role)
}

// UserFromContext 读取当前用户 ID 与角色，未登录时 ok=false。
func UserFromContext(ctx context.Context) (userID, role string, ok bool) {
	uid, ok1 := ctx.Value(userIDKey).(string)
	r, ok2 := ctx.Value(userRoleKey).(string)
	return uid, r, ok1 && ok2 && uid != ""
}
