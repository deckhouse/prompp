package task

import (
	"errors"
	"sync"

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
	errs    []error
	shardFn func(shard TShard) error
	wg      sync.WaitGroup
	done    prometheus.Counter
}

// NewGeneric init new [Generic].
func NewGeneric[TShard Shard](
	shardFn func(shard TShard) error,
	shardsNumber uint16,
	done prometheus.Counter,
) *Generic[TShard] {
	t := &Generic[TShard]{
		errs:    make([]error, shardsNumber),
		shardFn: shardFn,
		wg:      sync.WaitGroup{},
		done:    done,
	}

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
	done prometheus.Counter,
) {
	for i := range t.errs {
		t.errs[i] = nil
	}
	t.shardFn = shardFn
	t.done = done
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
	t.errs[shard.ShardID()] = t.shardFn(shard)
	t.wg.Done()
}

// Wait for the task to complete on all shards.
func (t *Generic[TShard]) Wait() error {
	t.wg.Wait()
	if t.done == nil {
		return errors.Join(t.errs...)
	}
	t.done.Inc()

	return errors.Join(t.errs...)
}
