package transactionhead

import (
	"context"
	"fmt"
	"runtime"

	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/head/poolprovider"
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
	headPool *poolprovider.HeadPool[TGShard]
}

// NewHead init new [Head].
func NewHead[TShard Shard, TGShard Shard](
	id string,
	shard TShard,
	gshard TGShard,
	headPool *poolprovider.HeadPool[TGShard],
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
func (*Head[TShard, TGShard]) AcquireQuery(context.Context) (func(), error) {
	return noopRelease, nil
}

// CreateTask creates a [task.Generic] for operations on the [Head] shards.
func (h *Head[TShard, TGShard]) CreateTask(_ string, shardFn func(shard TGShard) error) *task.Generic[TGShard] {
	t := h.headPool.GetTask()
	t.Reset(
		shardFn,
		nil,
	)

	return t
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

// ID returns the [Head] ID.
func (h *Head[TShard, TGShard]) ID() string {
	return h.id
}

// NumberOfShards returns current number of shards in to [Head].
func (*Head[TShard, TGShard]) NumberOfShards() uint16 {
	return 1
}

// PoolProvider returns the [poolprovider.HeadPool] for the [Head].
func (h *Head[TShard, TGShard]) PoolProvider() *poolprovider.HeadPool[TGShard] {
	return h.headPool
}

// PutTask adds [task.Generic] to the pool.
func (h *Head[TShard, TGShard]) PutTask(t *task.Generic[TGShard]) {
	h.headPool.PutTask(t)
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

// String serialize as string.
func (h *Head[TShard, TGShard]) String() string {
	return fmt.Sprintf("transaction_head{id: %s}", h.id)
}
