package querier

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/storage"
)

//
// Deduplicator
//

// Deduplicator accumulates and deduplicates incoming values.
type Deduplicator interface {
	// Add values to deduplicator by shard ID.
	Add(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string)

	// Values returns collected values.
	Values() []string
}

// deduplicatorCtor constructor [Deduplicator].
type deduplicatorCtor func(numberOfShards uint16) Deduplicator

//
// GenericTask
//

// Task the minimum required task [Generic] implementation.
type Task interface {
	// Wait for the task to complete on all shards.
	Wait() error
}

//
// DataStorage
//

// DataStorage the minimum required [DataStorage] implementation.
type DataStorage interface {
	// InstantQuery fills samples for instant query from data storage.
	InstantQuery(
		maxt int64,
		ids []uint32,
		samples uintptr,
	) cppbridge.DataStorageQueryResult

	// Query returns serialized chunks from data storage.
	Query(
		query cppbridge.DataStorageQuery,
	) cppbridge.DataStorageQueryResult

	// WithRLock calls fn on raw [cppbridge.DataStorage] with read lock.
	WithRLock(fn func(ds *cppbridge.DataStorage) error) error
}

//
// LSS
//

// LSS the minimum required [LSS] implementation.
type LSS interface {
	// QueryLabelNames returns all the unique label names present in lss in sorted order.
	QueryLabelNames(
		shardID uint16,
		matchers []model.LabelMatcher,
		dedupAdd func(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string),
	) error

	// QueryLabelValues query labels values to lss and add values to
	// the dedup-container that matches the given label matchers.
	QueryLabelValues(
		shardID uint16,
		name string,
		matchers []model.LabelMatcher,
		dedupAdd func(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string),
	) error

	// QuerySelector returns a created selector that matches the given label matchers.
	QuerySelector(shardID uint16, matchers []model.LabelMatcher) (uintptr, *cppbridge.LabelSetSnapshot, error)

	// WithRLock calls fn on raws [cppbridge.LabelSetStorage] with read lock.
	WithRLock(fn func(target, input *cppbridge.LabelSetStorage) error) error
}

//
// Shard
//

// Shard the minimum required head [Shard] implementation.
type Shard[TDataStorage DataStorage, TLSS LSS] interface {
	// DataStorage returns shard [DataStorage].
	DataStorage() TDataStorage

	// LSS returns shard labelset storage [LSS].
	LSS() TLSS

	// ShardID returns the shard ID.
	ShardID() uint16

	LoadAndQuerySeriesData() error

	LoadAndQuerySeriesDataTask() *shard.LoadAndQuerySeriesDataTask
}

//
// Head
//

// Head the minimum required [Head] implementation.
type Head[
	TGenericTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
] interface {
	// AcquireQuery acquires the [Head] semaphore with a weight of 1,
	// blocking until resources are available or ctx is done.
	// On success, returns nil. On failure, returns ctx.Err() and leaves the semaphore unchanged.
	AcquireQuery(ctx context.Context) (release func(), err error)

	// CreateTask create a task for operations on the [Head] shards.
	CreateTask(taskName string, shardFn func(s TShard) error) TGenericTask

	// Enqueue the task to be executed on shards [Head].
	Enqueue(t TGenericTask)

	// ReleaseTask to the pool.
	ReleaseTask(t TGenericTask)

	// EnqueueOnShard the task to be executed on head on specific shard.
	EnqueueOnShard(t TGenericTask, shardID uint16)

	// NumberOfShards returns current number of shards in to [Head].
	NumberOfShards() uint16

	// AcquireSnapshots gets a []*cppbridge.LabelSetSnapshot from the pool.
	AcquireSnapshots() []*cppbridge.LabelSetSnapshot

	// ReleaseSnapshots returns a []*cppbridge.LabelSetSnapshot to the pool after resetting it.
	ReleaseSnapshots(snapshots []*cppbridge.LabelSetSnapshot)

	// AcquireLSSQueryResults gets a []*cppbridge.LSSQueryResult from the pool.
	AcquireLSSQueryResults() []*cppbridge.LSSQueryResult

	// ReleaseLSSQueryResults returns a []*cppbridge.LSSQueryResult to the pool after resetting it.
	ReleaseLSSQueryResults(results []*cppbridge.LSSQueryResult)

	// AcquireSelectors gets a []uintptr from the pool.
	AcquireSelectors() []uintptr

	// ReleaseSelectors returns a []uintptr to the pool after resetting it.
	ReleaseSelectors(selectors []uintptr)

	// AcquireSeriesSet gets a []storage.SeriesSet from the pool.
	AcquireSeriesSet() []storage.SeriesSet

	// ReleaseSeriesSet returns a []storage.SeriesSet to the pool after resetting it.
	ReleaseSeriesSet(ssets []storage.SeriesSet)

	// AcquireChunkSeriesSet gets a []storage.ChunkSeriesSet from the pool.
	AcquireChunkSeriesSet() []storage.ChunkSeriesSet

	// ReleaseChunkSeriesSet returns a []storage.ChunkSeriesSet to the pool after resetting it.
	ReleaseChunkSeriesSet(csets []storage.ChunkSeriesSet)

	// AcquireSerializedData gets a []*cppbridge.DataStorageSerializedData from the pool.
	AcquireSerializedData() []*cppbridge.DataStorageSerializedData

	// ReleaseSerializedData returns a []*cppbridge.DataStorageSerializedData to the pool after resetting it.
	ReleaseSerializedData(sd []*cppbridge.DataStorageSerializedData)

	// AcquireErrors gets a []error from the pool.
	AcquireErrors() []error

	// ReleaseErrors returns a []error to the pool after resetting it.
	ReleaseErrors(errs []error)
}
