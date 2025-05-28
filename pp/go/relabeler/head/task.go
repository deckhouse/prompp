package head

import (
	"errors"
	"sync"

	"github.com/prometheus/prometheus/pp/go/relabeler"
)

//
// GenericTrueTask
//

// GenericTrueTask generic task, will be executed on each shard.
type GenericTrueTask struct {
	errs    []error
	shardFn relabeler.ShardFn
	wg      sync.WaitGroup
}

// NewGenericTrueTask init new GenericTrueTask.
func NewGenericTrueTask(shardFn relabeler.ShardFn, numberOfShards uint16) *GenericTrueTask {
	gt := &GenericTrueTask{
		errs:    make([]error, numberOfShards),
		shardFn: shardFn,
		wg:      sync.WaitGroup{},
	}
	gt.wg.Add(int(numberOfShards))

	return gt
}

// NewSingleTrueGenericTask init new GenericTrueTask for single shard.
func NewSingleTrueGenericTask(shardFn relabeler.ShardFn, numberOfShards uint16) *GenericTrueTask {
	gt := &GenericTrueTask{
		errs:    make([]error, numberOfShards),
		shardFn: shardFn,
		wg:      sync.WaitGroup{},
	}
	gt.wg.Add(1)

	return gt
}

// ExecuteOnShard execute task on shard.
func (t *GenericTrueTask) ExecuteOnShard(shard relabeler.Shard) {
	t.errs[shard.ShardID()] = t.shardFn(shard)
	t.wg.Done()
}

// Wait for the task to complete on all shards.
func (t *GenericTrueTask) Wait() error {
	t.wg.Wait()

	return errors.Join(t.errs...)
}
