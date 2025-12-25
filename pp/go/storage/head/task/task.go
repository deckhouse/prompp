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
	created, done, live, execute prometheus.Counter,
) *Generic[TShard] {
	t := &Generic[TShard]{
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

// NewTransactionGeneric init new [Generic] for transaction head.
func NewTransactionGeneric[TShard Shard](shardFn func(shard TShard) error) *Generic[TShard] {
	t := &Generic[TShard]{
		shardFn: shardFn,
		wg:      sync.WaitGroup{},
	}

	return t
}

// SetShardsNumber set shards number
func (t *Generic[TShard]) SetShardsNumber(number uint16) {
	t.errs = make([]error, number)
	t.wg.Add(int(number))
}

// ExecuteOnShard execute task on shard.
func (t *Generic[TShard]) ExecuteOnShard(shard TShard) {
	atomic.CompareAndSwapInt64(&t.executeTS, 0, time.Now().UnixMicro())
	if len(t.errs) == 1 {
		t.errs[0] = t.shardFn(shard)
	} else {
		t.errs[shard.ShardID()] = t.shardFn(shard)
	}

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
