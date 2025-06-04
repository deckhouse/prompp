package head

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/relabeler"
)

//
// GenericTask
//

// GenericTask generic task, will be executed on each shard.
type GenericTask struct {
	errs      []error
	shardFn   relabeler.ShardFn
	wg        sync.WaitGroup
	createdTS int64
	executeTS int64
	created   prometheus.Counter
	done      prometheus.Counter
	live      prometheus.Counter
	execute   prometheus.Counter
}

// NewGenericTask init new GenericTask.
func NewGenericTask(
	shardFn relabeler.ShardFn,
	created, done, live, execute prometheus.Counter,
	numberOfShards uint16,
) *GenericTask {
	t := &GenericTask{
		errs:      make([]error, numberOfShards),
		shardFn:   shardFn,
		wg:        sync.WaitGroup{},
		createdTS: time.Now().UnixMicro(),
		created:   created,
		done:      done,
		live:      live,
		execute:   execute,
	}
	t.wg.Add(int(numberOfShards))
	t.created.Inc()

	return t
}

// NewSingleGenericTask init new GenericTask for single shard.
func NewSingleGenericTask(
	shardFn relabeler.ShardFn,
	created, done, live, execute prometheus.Counter,
	numberOfShards uint16,
) *GenericTask {
	t := &GenericTask{
		errs:      make([]error, numberOfShards),
		wg:        sync.WaitGroup{},
		createdTS: time.Now().UnixMicro(),
		shardFn:   shardFn,
		created:   created,
		done:      done,
		live:      live,
		execute:   execute,
	}
	t.wg.Add(1)
	t.created.Inc()

	return t
}

// NewReadOnlyGenericTask init new GenericTask for read only head.
func NewReadOnlyGenericTask(
	shardFn relabeler.ShardFn,
	numberOfShards uint16,
) *GenericTask {
	t := &GenericTask{
		errs:    make([]error, numberOfShards),
		shardFn: shardFn,
		wg:      sync.WaitGroup{},
	}
	t.wg.Add(int(numberOfShards))

	return t
}

// ExecuteOnShard execute task on shard.
func (t *GenericTask) ExecuteOnShard(shard relabeler.Shard) {
	atomic.CompareAndSwapInt64(&t.executeTS, 0, time.Now().UnixMicro())

	t.errs[shard.ShardID()] = t.shardFn(shard)
	t.wg.Done()
}

// Wait for the task to complete on all shards.
func (t *GenericTask) Wait() error {
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
