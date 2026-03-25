package relabeler

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// ForLSSTask task for LSS.
	ForLSSTask = true
)

const (
	// LSSOutputRelabeling name of task.
	LSSOutputRelabeling = "lss_output_relabeling"
)

//
// GenericTask
//

// GenericTask generic task, will be executed on each shard.
type GenericTask struct {
	errs      []error
	shardFn   ShardFn
	wg        sync.WaitGroup
	createdTS int64
	executeTS int64
	created   prometheus.Counter
	done      prometheus.Counter
	live      prometheus.Counter
	execute   prometheus.Counter
	forLSS    bool
}

func NewGenericTask(
	shardFn ShardFn,
	created, done, live, execute prometheus.Counter,
	forLSS bool,
) *GenericTask {
	t := &GenericTask{
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
func NewReadOnlyGenericTask(shardFn ShardFn) *GenericTask {
	t := &GenericTask{
		shardFn: shardFn,
		wg:      sync.WaitGroup{},
	}

	return t
}

// SetShardsNumber set shards number
func (t *GenericTask) SetShardsNumber(number uint16) {
	t.errs = make([]error, number)
	t.wg.Add(int(number))
}

// ExecuteOnShard execute task on shard.
func (t *GenericTask) ExecuteOnShard(shard Shard) {
	atomic.CompareAndSwapInt64(&t.executeTS, 0, time.Now().UnixMicro())
	if len(t.errs) == 1 {
		t.errs[0] = t.shardFn(shard)
	} else {
		t.errs[shard.ShardID()] = t.shardFn(shard)
	}

	t.wg.Done()
}

// ForLSS indicates that the task is for operation on lss.
func (t *GenericTask) ForLSS() bool {
	return t.forLSS
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
