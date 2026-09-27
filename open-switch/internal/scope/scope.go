// Package scope carries the authenticated application boundary through call-control code.
package scope

import "context"

type applicationKey struct{}
type configVersionKey struct{}

// WithApplication binds a trusted application ID to a context.
func WithApplication(ctx context.Context, applicationID string) context.Context {
	return context.WithValue(ctx, applicationKey{}, applicationID)
}

// Application returns the authenticated application ID, or an empty string for unscoped internal work.
func Application(ctx context.Context) string {
	id, _ := ctx.Value(applicationKey{}).(string)
	return id
}

// WithConfigVersion pins configuration reads for an in-flight call.
func WithConfigVersion(ctx context.Context, version int64) context.Context {
	return context.WithValue(ctx, configVersionKey{}, version)
}

// ConfigVersion returns a pinned snapshot version, or zero for the active version.
func ConfigVersion(ctx context.Context) int64 {
	version, _ := ctx.Value(configVersionKey{}).(int64)
	return version
}
