package http

import (
	"context"
	"net/http"

	"open-switch/internal/scope"
)

// callMutationContext 合并 Idempotency-Key 与 body 中的 expected_version。
func callMutationContext(r *http.Request, expectedVersion int64) context.Context {
	ctx := r.Context()
	if expectedVersion > 0 {
		ctx = scope.WithExpectedVersion(ctx, expectedVersion)
	}
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		ctx = scope.WithIdempotency(ctx, key)
	}
	return ctx
}
