package scope

import "context"

// WithoutMutation 保留呼叫与配置上下文，但内部动作不重复消费入口命令的约束。
func WithoutMutation(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, expectedVersionKey{}, int64(0))
	return context.WithValue(ctx, idempotencyKey{}, "")
}
