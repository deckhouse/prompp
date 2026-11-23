package services

import (
	"runtime"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
)

func TestMetricsUpdaterKeepsMemoryMetricsUntilHeadIsCollected(t *testing.T) {
	// Arrange
	type testHead = head.Head[*metricsTestShard, *metricsTestShard]
	const memoryMetric = "prompp_head_cgo_memory_bytes"
	registry := prometheus.NewRegistry()
	h := head.NewHead(
		"rotated",
		[]*metricsTestShard{{}},
		func(s *metricsTestShard, _ uint16) *metricsTestShard { return s },
		nil,
		0,
		registry,
	)
	t.Cleanup(func() {
		if h != nil {
			require.NoError(t, h.Close())
		}
	})
	updater := NewMetricsUpdater[
		*task.Generic[*metricsTestShard], *metricsTestShard, *metricsTestShard, *testHead, HeadStatus,
	](nil, nil, nil, registry)
	updater.collectFromShards(h, false)

	// Act
	require.NoError(t, h.Close())

	// Assert
	runtime.GC()
	count, err := testutil.GatherAndCount(registry, memoryMetric)
	runtime.KeepAlive(h)
	require.NoError(t, err)
	require.Equal(t, 3, count, "a referenced closed head still owns memory")

	h = nil
	require.Eventually(t, func() bool {
		runtime.GC()
		count, err := testutil.GatherAndCount(registry, memoryMetric)

		return err == nil && count == 0
	}, 5*time.Second, 10*time.Millisecond, "rows for an unreachable head must be removed")
}

// metricsTestShard reports memory allocated for one shard.
type metricsTestShard struct {
	Shard
}

// Close closes the shard without any I/O.
func (*metricsTestShard) Close() error { return nil }

// DSAllocatedMemory reports the shard's data storage size.
func (*metricsTestShard) DSAllocatedMemory() uint64 { return 128 }

// LSSAllocatedMemory reports distinct sizes for the two LSS allocations.
func (*metricsTestShard) LSSAllocatedMemory() (uint64, uint64) { return 64, 16 }

// ShardID returns the ID for the only shard in the test.
func (*metricsTestShard) ShardID() uint16 { return 0 }
