package services

import (
	"runtime"
	"testing"
	"testing/synctest"
	"time"
	"weak"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/keeper"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
)

func TestMetricsUpdaterKeepsMemoryMetricsDuringHeadRemoval(t *testing.T) {
	const memoryMetric = "prompp_head_cgo_memory_bytes"
	registry := prometheus.NewRegistry()

	synctest.Test(t, func(t *testing.T) {
		// Arrange
		type testHead = head.Head[*metricsTestShard, *metricsTestShard]
		shard := &metricsTestShard{}
		h := head.NewHead(
			"rotated",
			[]*metricsTestShard{shard},
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
		removed := make(metricsHeadRemoved)
		heads := keeper.NewKeeper[testHead](1, removed)
		require.NoError(t, heads.Add(h, 0))

		updater := NewMetricsUpdater[
			*task.Generic[*metricsTestShard], *metricsTestShard, *metricsTestShard, *testHead, HeadStatus,
		](nil, nil, nil, registry)
		updater.collectFromShards(h, false)

		readingLSS := make(chan struct{})
		resume := make(chan struct{})
		collected := make(chan struct{})
		shard.beforeLSSRead = func() {
			close(readingLSS)
			<-resume
		}

		// Act
		go func(target *testHead) {
			updater.collectFromShards(target, false)
			close(collected)
		}(h)
		defer func() {
			close(resume)
			<-collected
		}()
		<-readingLSS

		headRef := weak.Make(h)
		go func(target *testHead) {
			heads.Remove([]*testHead{target})
		}(h)
		h = nil
		synctest.Wait()

		// Assert
		select {
		case <-removed:
		default:
			t.Fatal("head removal waited for the metrics collector")
		}
		require.Empty(t, heads.Heads())
		runtime.GC()
		require.True(t, headRef.Value() != nil, "the in-flight collector must keep the removed head reachable")

		count, err := testutil.GatherAndCount(registry, memoryMetric)
		require.NoError(t, err)
		require.Equal(t, 3, count, "removing the head must leave its memory metrics while collection is in flight")
	})

	require.Eventually(t, func() bool {
		runtime.GC()
		count, err := testutil.GatherAndCount(registry, memoryMetric)

		return err == nil && count == 0
	}, 5*time.Second, 10*time.Millisecond, "rows for an unreachable head must be removed")
}

// metricsHeadRemoved signals that the keeper removed and closed the head.
type metricsHeadRemoved chan struct{}

// Notify signals completion of the head removal.
func (removed metricsHeadRemoved) Notify() { close(removed) }

// metricsTestShard reports memory allocated for one shard.
type metricsTestShard struct {
	Shard
	beforeLSSRead func()
}

// Close closes the shard without any I/O.
func (*metricsTestShard) Close() error { return nil }

// DSAllocatedMemory reports the shard's data storage size.
func (*metricsTestShard) DSAllocatedMemory() uint64 { return 128 }

// LSSAllocatedMemory reports distinct sizes for the two LSS allocations.
func (s *metricsTestShard) LSSAllocatedMemory() (uint64, uint64) {
	if s.beforeLSSRead != nil {
		s.beforeLSSRead()
	}

	return 64, 16
}

// ShardID returns the ID for the only shard in the test.
func (*metricsTestShard) ShardID() uint16 { return 0 }
