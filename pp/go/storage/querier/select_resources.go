package querier

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/poolprovider"
	"github.com/prometheus/prometheus/pp/go/util/stagestats"
	"github.com/prometheus/prometheus/storage"
)

// selectResources owns pooled buffers and primary tasks until [Querier.Select] or [ChunkQuerier.Select] returns.
// Unacquired buffers are nil and are skipped during cleanup. Shared data-load tasks remain owned by their waiters.
type selectResources[TTask Task, TShard poolprovider.Shard] struct {
	pool *poolprovider.HeadPool[TShard]
	lss  queryLSSResources[TTask]
	// tasks holds the LSS selector task and, when reached, the primary DataStorage task.
	tasks     [2]TTask
	taskCount int

	snapshots       []*cppbridge.LabelSetSnapshot
	lssQueryResults []*cppbridge.LSSQueryResult
	serializedData  []*cppbridge.DataStorageSerializedData
	seriesSets      []storage.SeriesSet
	chunkSeriesSets []storage.ChunkSeriesSet
	slots           stagestats.ShardSlots
}

// newSelectResources binds the head pool without acquiring buffers, so cleanup can be deferred immediately.
func newSelectResources[TTask Task, TShard poolprovider.Shard](
	pool *poolprovider.HeadPool[TShard],
) selectResources[TTask, TShard] {
	return selectResources[TTask, TShard]{pool: pool}
}

// addTask records a primary task for return to the head pool after its execution has completed.
func (r *selectResources[TTask, TShard]) addTask(t TTask) {
	r.tasks[r.taskCount] = t
	r.taskCount++
}

// release returns acquired buffers and tasks to their pools after all tasks have completed.
// Query result buffers are closed before their pooled slice is cleared; returned series sets no longer need them.
func (r *selectResources[TTask, TShard]) release(putTask func(TTask)) {
	for i := r.taskCount - 1; i >= 0; i-- {
		putTask(r.tasks[i])
	}

	if r.chunkSeriesSets != nil {
		r.pool.PutChunkSeriesSet(r.chunkSeriesSets)
	}

	if r.seriesSets != nil {
		r.pool.PutSeriesSet(r.seriesSets)
	}

	if r.serializedData != nil {
		r.pool.PutSerializedData(r.serializedData)
	}

	if r.lss.errors != nil {
		r.pool.PutErrors(r.lss.errors)
	}

	if r.lss.selectors != nil {
		r.pool.PutSelectors(r.lss.selectors)
	}

	if r.lssQueryResults != nil {
		closeLSSQueryResults(r.lssQueryResults)
		r.pool.PutLSSQueryResults(r.lssQueryResults)
	}

	if r.snapshots != nil {
		r.pool.PutSnapshots(r.snapshots)
	}

	if r.slots != nil {
		r.pool.PutShardSlots(r.slots)
	}
}
