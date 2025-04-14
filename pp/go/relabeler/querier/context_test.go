package querier

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestInstantQueryContext(t *testing.T) {
	ctx := context.Background()
	require.False(t, InstantQueryFromContext(ctx))
	ctx = InstantQueryWithContext(ctx)
	require.True(t, InstantQueryFromContext(ctx))
}
