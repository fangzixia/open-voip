package switchapi

import "context"

type mutationKey struct{}

// Mutation 乐观锁与幂等键（由 BFF 或调用方写入 context）。
type Mutation struct {
	ExpectedVersion int64
	IdempotencyKey  string
}

func WithMutation(ctx context.Context, m Mutation) context.Context {
	return context.WithValue(ctx, mutationKey{}, m)
}

func mutationFrom(ctx context.Context) Mutation {
	v, _ := ctx.Value(mutationKey{}).(Mutation)
	return v
}

func applyMutation(ctx context.Context, body map[string]any) {
	m := mutationFrom(ctx)
	if m.ExpectedVersion > 0 {
		body["expected_version"] = m.ExpectedVersion
	}
}
