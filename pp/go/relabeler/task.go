package relabeler

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	ForLSSTask         = true
	ForDataStorageTask = false
	ExclusiveTask      = true
	NonExclusiveTask   = false
)

//
// GenericTask
//

// GenericTask generic task, will be executed on each shard.
type GenericTask struct {
	errs        []error
	shardFn     ShardFn
	wg          sync.WaitGroup
	createdTS   int64
	executeTS   int64
	created     prometheus.Counter
	done        prometheus.Counter
	live        prometheus.Counter
	execute     prometheus.Counter
	forLSS      bool
	isExclusive bool
}

func NewGenericTask(
	shardFn ShardFn,
	created, done, live, execute prometheus.Counter,
	numberOfShards uint16,
	forLSS, isExclusive bool,
) *GenericTask {
	t := &GenericTask{
		errs:        make([]error, numberOfShards),
		shardFn:     shardFn,
		wg:          sync.WaitGroup{},
		createdTS:   time.Now().UnixMicro(),
		created:     created,
		done:        done,
		live:        live,
		execute:     execute,
		forLSS:      forLSS,
		isExclusive: isExclusive,
	}
	t.wg.Add(int(numberOfShards))
	t.created.Inc()

	return t
}

// NewReadOnlyGenericTask init new GenericTask for read only head.
func NewReadOnlyGenericTask(
	shardFn ShardFn,
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
func (t *GenericTask) ExecuteOnShard(shard Shard) {
	atomic.CompareAndSwapInt64(&t.executeTS, 0, time.Now().UnixMicro())

	t.errs[shard.ShardID()] = t.shardFn(shard)
	t.wg.Done()
}

// ForLSS indicates that the task is for operation on lss.
func (t *GenericTask) ForLSS() bool {
	return t.forLSS
}

// IsExclusive indicates that the task is exclusive(write).
func (t *GenericTask) IsExclusive() bool {
	return t.isExclusive
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

//
// TaskWaiter
//

// TaskWaiter aggregates the wait for tasks to be completed.
type TaskWaiter struct {
	tasks []*GenericTask
}

// NewTaskWaiter init new TaskWaiter for n task.
func NewTaskWaiter(n int) *TaskWaiter {
	return &TaskWaiter{
		tasks: make([]*GenericTask, 0, n),
	}
}

// Add task to waiter.
func (tw *TaskWaiter) Add(t *GenericTask) {
	tw.tasks = append(tw.tasks, t)
}

// Wait for tasks to be completed.
func (tw *TaskWaiter) Wait() error {
	errs := make([]error, len(tw.tasks))
	for _, t := range tw.tasks {
		errs = append(errs, t.Wait())
	}

	return errors.Join(errs...)
}
