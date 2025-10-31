package transactionhead

import (
	"context"
	"fmt"
	"runtime"
	"sync"

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

type Head[TShard Shard, TGShard Shard] struct {
	id           string
	shard        TShard
	gshard       TGShard
	gshardLocker sync.Mutex
	// TODO gshard pool? concurency ring buffer
}

// NewHead init new [Head].
func NewHead[TShard Shard, TGShard Shard](
	id string,
	shard TShard,
	gshard TGShard,
) *Head[TShard, TGShard] {
	h := &Head[TShard, TGShard]{
		id:           id,
		shard:        shard,
		gshard:       gshard,
		gshardLocker: sync.Mutex{},
	}

	runtime.SetFinalizer(h, func(h *Head[TShard, TGShard]) {
		logger.Infof("[Head] %s destroyed", h.String())
	})

	logger.Infof("[Head] %s created", h.String())

	return h
}

// AcquireQuery implementation of the working [Head], no blocking.
func (h *Head[TShard, TGShard]) AcquireQuery(ctx context.Context) (func(), error) {
	return noopRelease, nil
}

// CreateTask create a task for operations on the [Head] shards.
func (h *Head[TShard, TGShard]) CreateTask(taskName string, shardFn func(shard TGShard) error) *task.Generic[TGShard] {
	return task.NewTransactionGeneric(shardFn)
}

// Enqueue the task to be executed on shards [Head].
func (h *Head[TShard, TGShard]) Enqueue(t *task.Generic[TGShard]) {
	t.SetShardsNumber(1)

	h.gshardLocker.Lock()
	t.ExecuteOnShard(h.gshard)
	h.gshardLocker.Unlock()
}

// EnqueueOnShard the task to be executed on head on specific shard.
func (h *Head[TShard, TGShard]) EnqueueOnShard(t *task.Generic[TGShard], _ uint16) {
	t.SetShardsNumber(1)

	h.gshardLocker.Lock()
	t.ExecuteOnShard(h.gshard)
	h.gshardLocker.Unlock()
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

// String serialize as string.
func (h *Head[TShard, TGShard]) String() string {
	return fmt.Sprintf("transaction_head{id: %s}", h.id)
}
