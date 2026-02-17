package transactionhead

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/zeropool"
)

// noopRelease do nothing, no locker.
func noopRelease() {}

//
// Shard
//

// Shard the minimum required head Shard implementation.
type Shard interface {
	// ShardID returns the shard ID.
	ShardID() uint16

	// Close closes the wal segmentWriter.
	Close() error
}

//
// HeadPool
//

// HeadPool pools for reusable objects.
type HeadPool struct {
	// use in appender
	shardedStateUpdatesPool    sync.Pool
	shardedRelabeledSeriesPool sync.Pool
	shardedInnerSeriesPool     sync.Pool
	taskPool                   sync.Pool
	statsPool                  zeropool.Pool[[]cppbridge.RelabelerStats]
	// use in querier
	snapshotsPool       zeropool.Pool[[]*cppbridge.LabelSetSnapshot]
	lssQueryResultsPool zeropool.Pool[[]*cppbridge.LSSQueryResult]
	selectorsPool       zeropool.Pool[[]uintptr]
	seriesSetPool       zeropool.Pool[[]storage.SeriesSet]
	chunkSeriesSetPool  zeropool.Pool[[]storage.ChunkSeriesSet]
	serializedDataPool  zeropool.Pool[[]*cppbridge.DataStorageSerializedData]
	errorsPool          zeropool.Pool[[]error]
}

// NewHeadPool init new [HeadPool], pools for reusable objects.
func NewHeadPool[TGShard Shard]() *HeadPool {
	return &HeadPool{
		shardedInnerSeriesPool: sync.Pool{
			New: func() any {
				return cppbridge.NewShardedInnerSeries(1)
			},
		},
		shardedRelabeledSeriesPool: sync.Pool{
			New: func() any {
				return cppbridge.NewShardedRelabeledSeries(1)
			},
		},
		shardedStateUpdatesPool: sync.Pool{
			New: func() any {
				return cppbridge.NewShardedStateUpdates(1)
			},
		},
		taskPool: sync.Pool{
			New: func() any {
				return task.NewGenericEmpty[TGShard](1)
			},
		},
		statsPool: zeropool.New(func() []cppbridge.RelabelerStats {
			return make([]cppbridge.RelabelerStats, 1)
		}),
		snapshotsPool: zeropool.New(func() []*cppbridge.LabelSetSnapshot {
			return make([]*cppbridge.LabelSetSnapshot, 1)
		}),
		lssQueryResultsPool: zeropool.New(func() []*cppbridge.LSSQueryResult {
			return make([]*cppbridge.LSSQueryResult, 1)
		}),
		selectorsPool: zeropool.New(func() []uintptr {
			return make([]uintptr, 1)
		}),
		seriesSetPool: zeropool.New(func() []storage.SeriesSet {
			return make([]storage.SeriesSet, 1)
		}),
		chunkSeriesSetPool: zeropool.New(func() []storage.ChunkSeriesSet {
			return make([]storage.ChunkSeriesSet, 1)
		}),
		serializedDataPool: zeropool.New(func() []*cppbridge.DataStorageSerializedData {
			return make([]*cppbridge.DataStorageSerializedData, 1)
		}),
		errorsPool: zeropool.New(func() []error {
			return make([]error, 1)
		}),
	}
}

//
// Head
//

// Head stores and manages shard, handles reads and writes of time series data for transaction operations.
// Append method are goroutine-unsafe.
type Head[TShard Shard, TGShard Shard] struct {
	id     string
	shard  TShard
	gshard TGShard
	// pools for reusable objects
	headPool *HeadPool
}

// NewHead init new [Head].
func NewHead[TShard Shard, TGShard Shard](
	id string,
	shard TShard,
	gshard TGShard,
	headPool *HeadPool,
) *Head[TShard, TGShard] {
	h := &Head[TShard, TGShard]{
		id:       id,
		shard:    shard,
		gshard:   gshard,
		headPool: headPool,
	}

	runtime.SetFinalizer(h, func(h *Head[TShard, TGShard]) {
		logger.Debugf("[Head] %s destroyed", h.String())
	})

	logger.Debugf("[Head] %s created", h.String())

	return h
}

// AcquireQuery implementation of the working [Head], no blocking.
func (*Head[TShard, TGShard]) AcquireQuery(ctx context.Context) (func(), error) {
	return noopRelease, nil
}

// CreateTask create a task for operations on the [Head] shards.
func (h *Head[TShard, TGShard]) CreateTask(taskName string, shardFn func(shard TGShard) error) *task.Generic[TGShard] {
	t := h.headPool.taskPool.Get().(*task.Generic[TGShard])
	t.Reset(
		shardFn,
		nil,
	)

	return t
}

// ReleaseTask to the pool.
func (h *Head[TShard, TGShard]) ReleaseTask(t *task.Generic[TGShard]) {
	h.headPool.taskPool.Put(t)
}

// Enqueue the task to be executed on shards [Head]. Method are goroutine-unsafe.
func (h *Head[TShard, TGShard]) Enqueue(t *task.Generic[TGShard]) {
	t.SetShardsNumber(1)

	t.ExecuteOnShard(h.gshard)
}

// EnqueueOnShard the task to be executed on head on specific shard. Method are goroutine-unsafe.
func (h *Head[TShard, TGShard]) EnqueueOnShard(t *task.Generic[TGShard], _ uint16) {
	t.SetShardsNumber(1)

	t.ExecuteOnShard(h.gshard)
}

// Generation returns current generation of [Head].
func (*Head[TShard, TGShard]) Generation() uint64 {
	return 0
}

// NumberOfShards returns current number of shards in to [Head].
func (*Head[TShard, TGShard]) NumberOfShards() uint16 {
	return 1
}

// RangeShards returns an iterator over the [Head] [Shard]s, through which the shard can be directly accessed.
func (h *Head[TShard, TGShard]) RangeShards() func(func(TShard) bool) {
	return func(yield func(s TShard) bool) {
		yield(h.shard)
	}
}

// Shards returns the [Head] [Shard]s.
func (h *Head[TShard, TGShard]) Shards() []TShard {
	return []TShard{h.shard}
}

// AcquireShardedInnerSeries gets a [cppbridge.ShardedInnerSeries] from the pool.
func (h *Head[TShard, TGShard]) AcquireShardedInnerSeries() *cppbridge.ShardedInnerSeries {
	return h.headPool.shardedInnerSeriesPool.Get().(*cppbridge.ShardedInnerSeries)
}

// ReleaseShardedInnerSeries returns a [cppbridge.ShardedInnerSeries] to the pool after resetting it.
func (h *Head[TShard, TGShard]) ReleaseShardedInnerSeries(s *cppbridge.ShardedInnerSeries) {
	s.Reset()
	h.headPool.shardedInnerSeriesPool.Put(s)
}

// AcquireShardedRelabeledSeries gets a [cppbridge.ShardedRelabeledSeries] from the pool.
func (h *Head[TShard, TGShard]) AcquireShardedRelabeledSeries() *cppbridge.ShardedRelabeledSeries {
	return h.headPool.shardedRelabeledSeriesPool.Get().(*cppbridge.ShardedRelabeledSeries)
}

// ReleaseShardedRelabeledSeries returns a [cppbridge.ShardedRelabeledSeries] to the pool after resetting it.
func (h *Head[TShard, TGShard]) ReleaseShardedRelabeledSeries(s *cppbridge.ShardedRelabeledSeries) {
	s.Reset()
	h.headPool.shardedRelabeledSeriesPool.Put(s)
}

// AcquireShardedStateUpdates gets a [cppbridge.ShardedStateUpdates] from the pool.
func (h *Head[TShard, TGShard]) AcquireShardedStateUpdates() *cppbridge.ShardedStateUpdates {
	return h.headPool.shardedStateUpdatesPool.Get().(*cppbridge.ShardedStateUpdates)
}

// ReleaseShardedStateUpdates returns a [cppbridge.ShardedStateUpdates] to the pool after resetting it.
func (h *Head[TShard, TGShard]) ReleaseShardedStateUpdates(s *cppbridge.ShardedStateUpdates) {
	s.Reset()
	h.headPool.shardedStateUpdatesPool.Put(s)
}

// AcquireRelabelerStats gets a []cppbridge.RelabelerStats from the pool.
func (h *Head[TShard, TGShard]) AcquireRelabelerStats() []cppbridge.RelabelerStats {
	return h.headPool.statsPool.Get()
}

// ReleaseRelabelerStats returns a []cppbridge.RelabelerStats to the pool after resetting it.
func (h *Head[TShard, TGShard]) ReleaseRelabelerStats(stats []cppbridge.RelabelerStats) {
	clear(stats)
	h.headPool.statsPool.Put(stats)
}

// AcquireSnapshots gets a []*cppbridge.LabelSetSnapshot from the pool.
func (h *Head[TShard, TGShard]) AcquireSnapshots() []*cppbridge.LabelSetSnapshot {
	return h.headPool.snapshotsPool.Get()
}

// ReleaseSnapshots returns a []*cppbridge.LabelSetSnapshot to the pool after resetting it.
func (h *Head[TShard, TGShard]) ReleaseSnapshots(snapshots []*cppbridge.LabelSetSnapshot) {
	clear(snapshots)
	h.headPool.snapshotsPool.Put(snapshots)
}

// AcquireLSSQueryResults gets a []*cppbridge.LSSQueryResult from the pool.
func (h *Head[TShard, TGShard]) AcquireLSSQueryResults() []*cppbridge.LSSQueryResult {
	return h.headPool.lssQueryResultsPool.Get()
}

// ReleaseLSSQueryResults returns a []*cppbridge.LSSQueryResult to the pool after resetting it.
func (h *Head[TShard, TGShard]) ReleaseLSSQueryResults(results []*cppbridge.LSSQueryResult) {
	clear(results)
	h.headPool.lssQueryResultsPool.Put(results)
}

// AcquireSelectors gets a []uintptr from the pool.
func (h *Head[TShard, TGShard]) AcquireSelectors() []uintptr {
	return h.headPool.selectorsPool.Get()
}

// ReleaseSelectors returns a []uintptr to the pool after resetting it.
func (h *Head[TShard, TGShard]) ReleaseSelectors(selectors []uintptr) {
	clear(selectors)
	h.headPool.selectorsPool.Put(selectors)
}

// AcquireSeriesSet gets a []storage.SeriesSet from the pool.
func (h *Head[TShard, TGShard]) AcquireSeriesSet() []storage.SeriesSet {
	return h.headPool.seriesSetPool.Get()
}

// ReleaseSeriesSet returns a []storage.SeriesSet to the pool after resetting it.
func (h *Head[TShard, TGShard]) ReleaseSeriesSet(ssets []storage.SeriesSet) {
	clear(ssets)
	h.headPool.seriesSetPool.Put(ssets)
}

// AcquireChunkSeriesSet gets a []storage.ChunkSeriesSet from the pool.
func (h *Head[TShard, TGShard]) AcquireChunkSeriesSet() []storage.ChunkSeriesSet {
	return h.headPool.chunkSeriesSetPool.Get()
}

// ReleaseChunkSeriesSet returns a []storage.ChunkSeriesSet to the pool after resetting it.
func (h *Head[TShard, TGShard]) ReleaseChunkSeriesSet(csets []storage.ChunkSeriesSet) {
	clear(csets)
	h.headPool.chunkSeriesSetPool.Put(csets)
}

// AcquireSerializedData gets a []*cppbridge.DataStorageSerializedData from the pool.
func (h *Head[TShard, TGShard]) AcquireSerializedData() []*cppbridge.DataStorageSerializedData {
	return h.headPool.serializedDataPool.Get()
}

// ReleaseSerializedData returns a []*cppbridge.DataStorageSerializedData to the pool after resetting it.
func (h *Head[TShard, TGShard]) ReleaseSerializedData(sd []*cppbridge.DataStorageSerializedData) {
	clear(sd)
	h.headPool.serializedDataPool.Put(sd)
}

// AcquireErrors gets a []error from the pool.
func (h *Head[TShard, TGShard]) AcquireErrors() []error {
	return h.headPool.errorsPool.Get()
}

// ReleaseErrors returns a []error to the pool after resetting it.
func (h *Head[TShard, TGShard]) ReleaseErrors(errs []error) {
	clear(errs)
	h.headPool.errorsPool.Put(errs)
}

// String serialize as string.
func (h *Head[TShard, TGShard]) String() string {
	return fmt.Sprintf("transaction_head{id: %s}", h.id)
}
