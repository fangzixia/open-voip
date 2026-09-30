// Package scope 在呼叫控制链路中传递配置快照版本。
package scope

import "context"

type configVersionKey struct{}

// WithConfigVersion 为进行中的通话固定配置快照版本，避免中途切换激活版本。
func WithConfigVersion(ctx context.Context, version int64) context.Context {
	return context.WithValue(ctx, configVersionKey{}, version)
}

// ConfigVersion 返回已固定的快照版本；为零表示使用当前激活版本。
func ConfigVersion(ctx context.Context) int64 {
	version, _ := ctx.Value(configVersionKey{}).(int64)
	return version
}
