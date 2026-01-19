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

// emprtyGenericTask init new empty GenericTask.
func emprtyGenericTask() *GenericTask {
	return &GenericTask{
		wg: sync.WaitGroup{},
	}
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

// SetShardsNumber set shards number.
func (t *GenericTask) SetShardsNumber(number uint16) {
	if cap(t.errs) < int(number) {
		t.errs = make([]error, number)
	} else {
		clear(t.errs[:cap(t.errs)])
		t.errs = t.errs[:number]
	}

	t.wg.Add(int(number))
}

// SingleSetShardsNumber set shards number for single shard.
func (t *GenericTask) SingleSetShardsNumber(number uint16) {
	if cap(t.errs) < int(number) {
		t.errs = make([]error, number)
	} else {
		clear(t.errs[:cap(t.errs)])
		t.errs = t.errs[:number]
	}

	t.wg.Add(1)
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
