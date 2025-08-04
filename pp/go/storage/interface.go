package storage

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

//
// Head
//

// Head implementation of the head with added metrics.
type Head interface {
	ID() string
	Generation() uint64
	Append(
		ctx context.Context,
		incomingData *IncomingData,
		state *cppbridge.State,
		relabelerID string,
		commitToWal bool,
	) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error)
	CommitToWal() error
	// MergeOutOfOrderChunks merge chunks with out of order data chunks.
	MergeOutOfOrderChunks()
	NumberOfShards() uint16
	Stop()
	Flush() error
	Reconfigure(ctx context.Context, inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error
	WriteMetrics(ctx context.Context)
	Status(limit int) HeadStatus
	Rotate() error
	Close() error
	Discard() error
	String() string
	CopySeriesFrom(other Head)
	Enqueue(t *GenericTask)
	EnqueueOnShard(t *GenericTask, shardID uint16)
	CreateTask(taskName string, fn ShardFn, isLss bool) *GenericTask
	Concurrency() int64
	RLockQuery(ctx context.Context) (runlock func(), err error)
	Raw() Head
}

//
// Shard
//

// Shard interface for shards [Head].
type Shard interface {
	// DataStorage returns [DataStorage] shard.
	DataStorage() DataStorage
	// lock for DataStorage
	DataStorageLock()
	DataStorageRLock()
	DataStorageRUnlock()
	DataStorageUnlock()
	// LSS returns [LSS] shard.
	LSS() LSS
	// lock for LSS
	LSSLock()
	LSSRLock()
	LSSRUnlock()
	LSSUnlock()
	// ShardID returns ID shard.
	ShardID() uint16
	// Wal returns [Wal] shard.
	Wal() Wal
}

// ShardFn function executing on a [Shard].
type ShardFn func(shard Shard) error

//
// DataStorage
//

// DataStorage sample storage interface.
type DataStorage interface {
	AllocatedMemory() uint64
	// AppendInnerSeriesSlice append slice of [cppbridge.InnerSeries](samples with label IDs) to the storage.
	AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries)
	InstantQuery(targetTimestamp, notFoundValueTimestampValue int64, seriesIDs []uint32) []cppbridge.Sample
	MergeOutOfOrderChunks()
	Query(query cppbridge.HeadDataStorageQuery) *cppbridge.HeadDataStorageSerializedChunks
	Raw() *cppbridge.HeadDataStorage
}

//
// LSS
//

// LSS labelset storage interface.
type LSS interface {
	AllocatedMemory() uint64
	GetLabelSets(labelSetIDs []uint32) *cppbridge.LabelSetStorageGetLabelSetsResult
	GetSnapshot() *cppbridge.LabelSetSnapshot
	Input() *cppbridge.LabelSetStorage
	QueryLabelNames(matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelNamesResult
	QueryLabelValues(label_name string, matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelValuesResult
	QuerySelector(matchers []model.LabelMatcher) (selector uintptr, status uint32)
	Raw() *cppbridge.LabelSetStorage
	ResetSnapshot()
	Target() *cppbridge.LabelSetStorage
}

//
// Wal
//

// Wal write-ahead log for [Shard].
type Wal interface {
	// DO NOT USE in public interfaces like ForEachShard
	Commit() error
	Flush() error
	Write(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error)
}

//
// MetricData
//

// MetricData is an universal interface for blob protobuf data or batch [model.TimeSeries].
type MetricData interface {
	// Destroy incoming data.
	Destroy()
}

//
// ProtobufData
//

// ProtobufData is an universal interface for blob protobuf data.
type ProtobufData interface {
	Bytes() []byte
	Destroy()
}

//
// TimeSeriesData
//

// TimeSeriesBatch is an universal interface for batch [model.TimeSeries].
type TimeSeriesBatch interface {
	TimeSeries() []model.TimeSeries
	Destroy()
}
