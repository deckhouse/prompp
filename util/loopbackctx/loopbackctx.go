package loopbackctx

import (
	"context"
	"time"
)

// loopbackParam key for loopback duration in context.
type loopbackParam struct{}

// ContextWithLoopback append loopback duration to context.
func ContextWithLoopback(ctx context.Context, lookbackDelta time.Duration) context.Context {
	return context.WithValue(ctx, loopbackParam{}, lookbackDelta)
}

// LoopbackFromContext extract loopback duration from context.
func LoopbackFromContext(ctx context.Context) time.Duration {
	lookbackDelta, ok := ctx.Value(loopbackParam{}).(time.Duration)
	if !ok {
		return time.Duration(0)
	}

	return lookbackDelta
}
