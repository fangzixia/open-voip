package scope

import "context"

type idempotencyKey struct{}

// WithIdempotency 将 HTTP Idempotency-Key 写入上下文。
func WithIdempotency(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, idempotencyKey{}, key)
}

// Idempotency 读取请求幂等键。
func Idempotency(ctx context.Context) string {
	v, _ := ctx.Value(idempotencyKey{}).(string)
	return v
}
