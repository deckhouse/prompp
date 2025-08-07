package task

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
)

//
// Shard
//

// Shard the minimum required head Shard implementation.
type Shard interface {
	QueryLabelValues(
		name string,
		matchers []model.LabelMatcher,
		dedupAdd func(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string),
	) error
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
	forLSS    bool
}

// NewGenericTask init new [Generic].
func NewGenericTask(
	shardFn func(shard TShard) error,
	created, done, live, execute prometheus.Counter,
	forLSS bool,
) *Generic {
	t := &Generic{
		shardFn:   shardFn,
		wg:        sync.WaitGroup{},
		createdTS: time.Now().UnixMicro(),
		created:   created,
		done:      done,
		live:      live,
		execute:   execute,
		forLSS:    forLSS,
	}
	t.created.Inc()

	return t
}

// NewReadOnlyGenericTask init new GenericTask for read only head.
func NewReadOnlyGenericTask(shardFn func(shard TShard) error) *Generic {
	t := &Generic{
		shardFn: shardFn,
		wg:      sync.WaitGroup{},
	}

	return t
}

// SetShardsNumber set shards number
func (t *Generic) SetShardsNumber(number uint16) {
	t.errs = make([]error, number)
	t.wg.Add(int(number))
}

// ExecuteOnShard execute task on shard.
func (t *Generic) ExecuteOnShard(shard Shard) {
	atomic.CompareAndSwapInt64(&t.executeTS, 0, time.Now().UnixMicro())
	t.errs[shard.ShardID()] = t.shardFn(shard)
	t.wg.Done()
}

// Wait for the task to complete on all shards.
func (t *Generic) Wait() error {
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
