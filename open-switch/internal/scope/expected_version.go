package scope

import "context"

type expectedVersionKey struct{}

// WithExpectedVersion 写入客户端乐观锁版本（0 表示不校验）。
func WithExpectedVersion(ctx context.Context, version int64) context.Context {
	if version <= 0 {
		return ctx
	}
	return context.WithValue(ctx, expectedVersionKey{}, version)
}

// ExpectedVersion 读取 expected_version。
func ExpectedVersion(ctx context.Context) int64 {
	v, _ := ctx.Value(expectedVersionKey{}).(int64)
	return v
}
