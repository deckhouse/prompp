package task

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

//
// Shard
//

// Shard the minimum required head Shard implementation.
type Shard interface {
	ShardID() uint16
}

//
// GenericTask
//

// Generic generic task, will be executed on each shard.
type Generic[TShard Shard] struct {
	errs      []error
	shardFn   func(shard TShard) error
	wg        sync.WaitGroup
	createdTS int64
	executeTS int64
	created   prometheus.Counter
	done      prometheus.Counter
	live      prometheus.Counter
	execute   prometheus.Counter
}

// NewGeneric init new [Generic].
func NewGeneric[TShard Shard](
	shardFn func(shard TShard) error,
	shardsNumber uint16,
	created, done, live, execute prometheus.Counter,
) *Generic[TShard] {
	t := &Generic[TShard]{
		errs:      make([]error, shardsNumber),
		shardFn:   shardFn,
		wg:        sync.WaitGroup{},
		createdTS: time.Now().UnixMicro(),
		created:   created,
		done:      done,
		live:      live,
		execute:   execute,
	}
	t.created.Inc()

	return t
}

// NewGenericEmpty init new empty [Generic] for pooling.
func NewGenericEmpty[TShard Shard](shardsNumber uint16) *Generic[TShard] {
	return &Generic[TShard]{
		errs: make([]error, shardsNumber),
		wg:   sync.WaitGroup{},
	}
}

// Reset resets the task for reuse from pool.
func (t *Generic[TShard]) Reset(
	shardFn func(shard TShard) error,
	created, done, live, execute prometheus.Counter,
) {
	for i := range t.errs {
		t.errs[i] = nil
	}
	t.shardFn = shardFn
	t.createdTS = time.Now().UnixMicro()
	t.executeTS = 0
	t.created = created
	t.done = done
	t.live = live
	t.execute = execute
	t.created.Inc()
}

// NewTransactionGeneric init new [Generic] for transaction head.
func NewTransactionGeneric[TShard Shard](shardFn func(shard TShard) error) *Generic[TShard] {
	t := &Generic[TShard]{
		errs:    make([]error, 1),
		shardFn: shardFn,
		wg:      sync.WaitGroup{},
	}

	return t
}

// SetShardsNumber set shards number
func (t *Generic[TShard]) SetShardsNumber(number uint16) {
	t.wg.Add(int(number))
}

// ExecuteOnShard execute task on shard.
func (t *Generic[TShard]) ExecuteOnShard(shard TShard) {
	atomic.CompareAndSwapInt64(&t.executeTS, 0, time.Now().UnixMicro())
	t.errs[shard.ShardID()] = t.shardFn(shard)
	t.wg.Done()
}

// Wait for the task to complete on all shards.
func (t *Generic[TShard]) Wait() error {
	t.wg.Wait()
	if t.done == nil {
		return errors.Join(t.errs...)
	}

	now := time.Now().UnixMicro()
	t.done.Inc()
	t.execute.Add(float64(now - t.executeTS))
	t.live.Add(float64(now - t.createdTS))

	return errors.Join(t.errs...)
}
