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
	// ForDataStorageTask task for DataStorage.
	ForDataStorageTask = false
	// ExclusiveTask exclusive task(write).
	ExclusiveTask = true
	// NonExclusiveTask non-exclusive task(read).
	NonExclusiveTask = false
)

const (
	// LSSInputRelabeling name of task.
	LSSInputRelabeling = "lss_input_relabeling"
	// LSSAppendRelabelerSeries name of task.
	LSSAppendRelabelerSeries = "lss_append_relabeler_series"

	// LSSWalCommit name of task.
	LSSWalCommit = "lss_wal_commit"
	// LSSWalFlush name of task.
	LSSWalFlush = "lss_wal_flush"
	// LSSWalWrite name of task.
	LSSWalWrite = "lss_wal_write"

	// LSSCopyAddedSeries name of task.
	LSSCopyAddedSeries = "lss_copy_added_series"

	// LSSOutputRelabeling name of task.
	LSSOutputRelabeling = "lss_output_relabeling"

	// LSSAllocatedMemory name of task.
	LSSAllocatedMemory = "lss_allocated_memory"

	// LSSHeadStatus name of task.
	LSSHeadStatus = "lss_head_status"

	// LSSQueryChunkQuerier name of task.
	LSSQueryChunkQuerier = "lss_query_chunk_querier"
	// LSSLabelValuesChunkQuerier name of task.
	LSSLabelValuesChunkQuerier = "lss_label_values_chunk_querier"
	// LSSLabelNamesChunkQuerier name of task.
	LSSLabelNamesChunkQuerier = "lss_label_names_chunk_querier"

	// LSSQueryInstantQuerier name of task.
	LSSQueryInstantQuerier = "lss_query_instant_querier"
	// LSSQueryRangeQuerier name of task.
	LSSQueryRangeQuerier = "lss_query_range_querier"
	// LSSLabelValuesQuerier name of task.
	LSSLabelValuesQuerier = "lss_label_values_querier"
	// LSSLabelNamesQuerier name of task.
	LSSLabelNamesQuerier = "lss_label_names_querier"

	// DSAppendInnerSeries name of task.
	DSAppendInnerSeries = "data_storage_append_inner_series"
	// DSMergeOutOfOrderChunks name of task.
	DSMergeOutOfOrderChunks = "data_storage_merge_out_of_order_chunks"

	// DSAllocatedMemory name of task.
	DSAllocatedMemory = "data_storage_allocated_memory"

	// DSUnload
	DSUnload = "data_storage_unload"

	// DSHeadStatus name of task.
	DSHeadStatus = "data_storage_head_status"

	// DSQueryChunkQuerier name of task.
	DSQueryChunkQuerier = "data_storage_query_chunk_querier"

	// DSQueryInstantQuerier name of task.
	DSQueryInstantQuerier = "data_storage_query_instant_querier"
	// DSQueryRangeQuerier name of task.
	DSQueryRangeQuerier = "data_storage_query_range_querier"

	// Read Only

	// BlockWrite name of task.
	BlockWrite = "block_write"
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

// ExecuteOnShardWithLocker execute task on shard with locker.
func (t *GenericTask) ExecuteOnShardWithLocker(shard Shard, lock, unlock func()) {
	lock()
	atomic.CompareAndSwapInt64(&t.executeTS, 0, time.Now().UnixMicro())
	t.errs[shard.ShardID()] = t.shardFn(shard)
	unlock()
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
