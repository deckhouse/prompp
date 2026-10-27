package querier

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/poolprovider"
	"github.com/prometheus/prometheus/pp/go/util/stagestats"
	"github.com/prometheus/prometheus/storage"
)

// taskPool returns completed tasks to the head that created them.
// Heads implement this interface without creating a bound method for each query.
type taskPool[TTask Task] interface {
	// PutTask returns a completed task to its owner's pool.
	PutTask(TTask)
}

// queryResources owns pooled buffers, primary tasks, semaphore access, and timing for a [Querier] or [ChunkQuerier] call.
// Unacquired buffers are nil and are skipped during cleanup. Shared data-load tasks remain owned by their waiters.
type queryResources[TTask Task, TShard poolprovider.Shard] struct {
	pool *poolprovider.HeadPool[TShard]
	// tasksPool returns primary tasks through the owning head.
	tasksPool taskPool[TTask]
	// tasks holds up to two primary tasks; metadata queries acquire only one.
	tasks     [2]TTask
	taskCount int
	// stats records working stages and the legacy duration through semaphore release.
	stats queryStats
	// releaseQuery releases acquired head query access after temporary resources have been returned.
	releaseQuery func()

	// selectors holds per-shard selector pointers until the caller's cleanup.
	selectors []uintptr
	// errors holds per-shard snapshot query errors until the caller's cleanup.
	errors []error

	snapshots       []*cppbridge.LabelSetSnapshot
	lssQueryResults []*cppbridge.LSSQueryResult
	serializedData  []*cppbridge.DataStorageSerializedData
	seriesSets      []storage.SeriesSet
	chunkSeriesSets []storage.ChunkSeriesSet
	slots           stagestats.ShardSlots
}

// newQueryResources binds acquired head query access, timing, and pools without acquiring buffers.
// The caller defers [queryResources.close] before acquiring tasks or buffers.
func newQueryResources[TTask Task, TShard poolprovider.Shard](
	pool *poolprovider.HeadPool[TShard],
	tasksPool taskPool[TTask],
	stats queryStats,
	releaseQuery func(),
) queryResources[TTask, TShard] {
	return queryResources[TTask, TShard]{pool: pool, tasksPool: tasksPool, stats: stats, releaseQuery: releaseQuery}
}

// addTask records a primary task for return to the head pool after its execution has completed.
func (r *queryResources[TTask, TShard]) addTask(t TTask) {
	r.tasks[r.taskCount] = t
	r.taskCount++
}

// close returns acquired buffers and tasks, releases head query access, and then observes the legacy histogram.
// Query result buffers are closed before their pooled slice is cleared; returned series sets no longer need them.
func (r *queryResources[TTask, TShard]) close() {
	for i := r.taskCount - 1; i >= 0; i-- {
		r.tasksPool.PutTask(r.tasks[i])
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

	if r.errors != nil {
		r.pool.PutErrors(r.errors)
	}

	if r.selectors != nil {
		r.pool.PutSelectors(r.selectors)
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

	r.releaseQuery()
	r.stats.observeDuration()
}

// beginSelectQuery starts timing and acquires head query access, including timing failed acquisition attempts.
// After successful acquisition, the caller defers [queryResources.close] before acquiring buffers or tasks.
func beginSelectQuery[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
](
	ctx context.Context,
	head THead,
	metrics *Metrics,
	operation queryOperation,
) (resources queryResources[TTask, TShard], err error) {
	resources.stats = metrics.startQuery(operation)
	resources.releaseQuery, err = head.AcquireQuery(ctx)
	resources.stats.lap.Mark(stageSemaphoreWait)

	if err != nil {
		return resources, err
	}

	resources.pool = head.PoolProvider()
	resources.tasksPool = head

	return resources, nil
}
