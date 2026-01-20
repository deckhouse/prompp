package transactionhead

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
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
// Head
//

// Head stores and manages shard, handles reads and writes of time series data for transaction operations.
// Append method are goroutine-unsafe.
type Head[TShard Shard, TGShard Shard] struct {
	id     string
	shard  TShard
	gshard TGShard

	// pools for reusable objects
	shardedInnerSeriesPool     sync.Pool
	shardedRelabeledSeriesPool sync.Pool
	shardedStateUpdatesPool    sync.Pool
}

// NewHead init new [Head].
func NewHead[TShard Shard, TGShard Shard](
	id string,
	shard TShard,
	gshard TGShard,
) *Head[TShard, TGShard] {
	h := &Head[TShard, TGShard]{
		id:     id,
		shard:  shard,
		gshard: gshard,
		// pools for reusable objects
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
func (*Head[TShard, TGShard]) CreateTask(taskName string, shardFn func(shard TGShard) error) *task.Generic[TGShard] {
	return task.NewTransactionGeneric(shardFn)
}

// ReleaseTask to the pool.
func (*Head[TShard, TGShard]) ReleaseTask(_ *task.Generic[TGShard]) {}

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

// AcquireShardedInnerSeries gets a [cppbridge.ShardedInnerSeries] from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireShardedInnerSeries() *cppbridge.ShardedInnerSeries {
	return h.shardedInnerSeriesPool.Get().(*cppbridge.ShardedInnerSeries)
}

// ReleaseShardedInnerSeries returns a [cppbridge.ShardedInnerSeries] to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseShardedInnerSeries(s *cppbridge.ShardedInnerSeries) {
	s.Reset()
	h.shardedInnerSeriesPool.Put(s)
}

// AcquireShardedRelabeledSeries gets a [cppbridge.ShardedRelabeledSeries] from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireShardedRelabeledSeries() *cppbridge.ShardedRelabeledSeries {
	return h.shardedRelabeledSeriesPool.Get().(*cppbridge.ShardedRelabeledSeries)
}

// ReleaseShardedRelabeledSeries returns a [cppbridge.ShardedRelabeledSeries] to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseShardedRelabeledSeries(s *cppbridge.ShardedRelabeledSeries) {
	s.Reset()
	h.shardedRelabeledSeriesPool.Put(s)
}

// AcquireShardedStateUpdates gets a [cppbridge.ShardedStateUpdates] from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireShardedStateUpdates() *cppbridge.ShardedStateUpdates {
	return h.shardedStateUpdatesPool.Get().(*cppbridge.ShardedStateUpdates)
}

// ReleaseShardedStateUpdates returns a [cppbridge.ShardedStateUpdates] to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseShardedStateUpdates(s *cppbridge.ShardedStateUpdates) {
	s.Reset()
	h.shardedStateUpdatesPool.Put(s)
}

// String serialize as string.
func (h *Head[TShard, TGShard]) String() string {
	return fmt.Sprintf("transaction_head{id: %s}", h.id)
}
