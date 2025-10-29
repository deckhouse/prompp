package querier

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
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
// Task
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
	// InstantQuery returns samples for instant query from data storage.
	InstantQuery(
		maxt, valueNotFoundTimestampValue int64,
		ids []uint32,
	) ([]cppbridge.Sample, cppbridge.DataStorageQueryResult)

	// Query returns serialized chunks from data storage.
	Query(
		query cppbridge.HeadDataStorageQuery,
	) cppbridge.DataStorageQueryResult

	// WithRLock calls fn on raw [cppbridge.HeadDataStorage] with read lock.
	WithRLock(fn func(ds *cppbridge.HeadDataStorage) error) error
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
// GShard
//

// GShard the minimum required head [PerGoroutineShard] implementation.
type GShard[TDataStorage DataStorage, TLSS LSS] interface {
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
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TGShard GShard[TDataStorage, TLSS],
] interface {
	// AcquireQuery acquires the [Head] semaphore with a weight of 1,
	// blocking until resources are available or ctx is done.
	// On success, returns nil. On failure, returns ctx.Err() and leaves the semaphore unchanged.
	AcquireQuery(ctx context.Context) (release func(), err error)

	// CreateTask create a task for operations on the [Head] shards.
	CreateTask(taskName string, shardFn func(s TGShard) error) TTask

	// Enqueue the task to be executed on shards [Head].
	Enqueue(t TTask)

	// EnqueueOnShard the task to be executed on head on specific shard.
	EnqueueOnShard(t TTask, shardID uint16)

	// NumberOfShards returns current number of shards in to [Head].
	NumberOfShards() uint16

	// IsReadOnly returns true if the [Head] has switched to read-only.
	IsReadOnly() bool
}
