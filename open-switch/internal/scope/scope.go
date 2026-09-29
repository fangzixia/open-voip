// Package scope 在呼叫控制链路中传递已认证的应用边界与配置快照版本。
package scope

import "context"

type applicationKey struct{}
type configVersionKey struct{}

// WithApplication 将受信任的业务应用 ID 绑定到上下文。
func WithApplication(ctx context.Context, applicationID string) context.Context {
	return context.WithValue(ctx, applicationKey{}, applicationID)
}

// Application 返回已认证应用 ID；未绑定作用域时为空（内部任务）。
func Application(ctx context.Context) string {
	id, _ := ctx.Value(applicationKey{}).(string)
	return id
}

// WithConfigVersion 为进行中的通话固定配置快照版本，避免中途切换激活版本。
func WithConfigVersion(ctx context.Context, version int64) context.Context {
	return context.WithValue(ctx, configVersionKey{}, version)
}

// ConfigVersion 返回已固定的快照版本；为零表示使用当前激活版本。
func ConfigVersion(ctx context.Context) int64 {
	version, _ := ctx.Value(configVersionKey{}).(int64)
	return version
}
