package poolprovider_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/storage/head/poolprovider"
)

// testShard implementation [poolprovider.Shard].
type testShard struct{}

// ShardID implementation [poolprovider.Shard].
func (*testShard) ShardID() uint16 {
	return 0
}

func TestHeadPoolShardSlots(t *testing.T) {
	// Arrange
	pool := poolprovider.NewHeadPool[*testShard](3)
	slots := pool.GetShardSlots()
	slots.Set(2, 1, 10)

	// Act
	pool.PutShardSlots(slots)
	reused := pool.GetShardSlots()

	// Assert
	require.Len(t, reused, 3)
	require.Zero(t, reused.Max(1))
}
