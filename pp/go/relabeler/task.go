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

	// LSSQueryChunkQuerySelector name of task.
	LSSQueryChunkQuerySelector = "lss_query_chunk_query_selector"
	// LSSLabelValuesChunkQuerier name of task.
	LSSLabelValuesChunkQuerier = "lss_label_values_chunk_querier"
	// LSSLabelNamesChunkQuerier name of task.
	LSSLabelNamesChunkQuerier = "lss_label_names_chunk_querier"

	// LSSQueryInstantQuerySelector name of task.
	LSSQueryInstantQuerySelector = "lss_query_instant_query_selector"
	// LSSQueryRangeQuerySelector name of task.
	LSSQueryRangeQuerySelector = "lss_query_range_query_selector"
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

	// DSHeadStatus name of task.
	DSHeadStatus = "data_storage_head_status"

	// DSQueryChunkQuerier name of task.
	DSQueryChunkQuerier = "data_storage_query_chunk_querier"

	// DSQueryInstantQuerier name of task.
	DSQueryInstantQuerier = "data_storage_query_instant_querier"
	// DSQueryRangeQuerier name of task.
	DSQueryRangeQuerier = "data_storage_query_range_querier"

	// DSUnloadUnusedSeriesData name of task.
	DSUnloadUnusedSeriesData = "data_storage_unload_unused_series_data"

	// DSLoadUnusedSeriesDataAndQuery name of task.
	DSLoadUnusedSeriesDataAndQuery = "data_storage_load_unused_series_data_and_query"

	// Read Only

	// BlockWrite name of task.
	BlockWrite = "block_write"
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

//
// TaskWaiter
//

// TaskWaiter aggregates the wait for tasks to be completed.
type TaskWaiter struct {
	tasks []*GenericTask
}

// NewTaskWaiter init new TaskWaiter for n task.
func NewTaskWaiter(n int) TaskWaiter {
	return TaskWaiter{
		tasks: make([]*GenericTask, 0, n),
	}
}

// Add task to waiter.
func (tw *TaskWaiter) Add(t *GenericTask) {
	tw.tasks = append(tw.tasks, t)
}

// Wait for tasks to be completed.
func (tw *TaskWaiter) Wait() error {
	var err error
	for _, t := range tw.tasks {
		err = errors.Join(err, t.Wait())
	}

	return err
}
