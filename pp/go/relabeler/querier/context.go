package querier

import "context"

type instantQueryContextKey struct{}

func InstantQueryWithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, instantQueryContextKey{}, struct{}{})
}

func InstantQueryFromContext(ctx context.Context) bool {
	return ctx.Value(instantQueryContextKey{}) != nil
}
