package relabeler

import (
	"context"
	"errors"
	"hash/crc32"
	"sync/atomic"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

// DataStorage - data storage interface.
type DataStorage interface {
	AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries)
	Raw() *cppbridge.HeadDataStorage
	MergeOutOfOrderChunks()
	Query(query cppbridge.HeadDataStorageQuery) (*cppbridge.HeadDataStorageSerializedChunks, cppbridge.DataStorageQueryResult)
	QueryFinal(queriers []uintptr)
	InstantQuery(targetTimestamp, notFoundValueTimestampValue int64, seriesIDs []uint32) ([]cppbridge.Sample, cppbridge.DataStorageQueryResult)
	AllocatedMemory() uint64
	CreateUnusedSeriesDataUnloader() *cppbridge.UnusedSeriesDataUnloader
	CreateLoader(queriers []uintptr) *cppbridge.UnloadedDataLoader
	CreateRevertableLoader(lss *cppbridge.LabelSetStorage, lsIdBatchSize uint32) *cppbridge.UnloadedDataRevertableLoader
	TimeInterval() cppbridge.TimeInterval
	GetQueriedSeriesBitset() []byte
}

type LSS interface {
	Raw() *cppbridge.LabelSetStorage
	AllocatedMemory() uint64
	QueryLabelValues(labelName string, matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelValuesResult
	QueryLabelNames(matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelNamesResult
	QuerySelector(matchers []model.LabelMatcher) (selector uintptr, status uint32)
	GetLabelSets(labelSetIDs []uint32) *cppbridge.LabelSetStorageGetLabelSetsResult
	GetSnapshot() *cppbridge.LabelSetSnapshot
	ResetSnapshot()
	Input() *cppbridge.LabelSetStorage
	Target() *cppbridge.LabelSetStorage
}

type Wal interface {
	Write(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error)
	// DO NOT USE in public interfaces like ForEachShard
	Commit() error
	Flush() error
}

type UnloadedDataSnapshotHeader struct {
	Crc32        uint32
	SnapshotSize uint32
}

func NewUnloadedDataSnapshotHeader(snapshot []byte) UnloadedDataSnapshotHeader {
	return UnloadedDataSnapshotHeader{Crc32: crc32.ChecksumIEEE(snapshot), SnapshotSize: uint32(len(snapshot))}
}

func (h UnloadedDataSnapshotHeader) IsValid(snapshot []byte) bool {
	return h.Crc32 == crc32.ChecksumIEEE(snapshot)
}

type UnloadedDataStorage interface {
	WriteSnapshot(snapshot []byte) (UnloadedDataSnapshotHeader, error)
	WriteIndex(UnloadedDataSnapshotHeader)
	ForEachSnapshot(f func(snapshot []byte, isLast bool)) error
}

type QueriedSeriesStorage interface {
	Write(queriedSeriesBitset []byte, timestamp int64) error
	Close() error
}

type DataStorageLoadAndQueryTask interface {
	Release() []uintptr
}

type InputRelabeler interface {
	CacheAllocatedMemory() uint64
}

// Shard interface.
type Shard interface {
	ShardID() uint16
	DataStorage() DataStorage
	LSS() LSS
	Wal() Wal
	UnloadedDataStorage() UnloadedDataStorage
	QueriedSeriesStorage() QueriedSeriesStorage
	LoadAndQueryTask() DataStorageLoadAndQueryTask
	// lock for DataStorage
	DataStorageLock()
	DataStorageRLock()
	DataStorageRUnlock()
	DataStorageUnlock()
	// lock for LSS
	LSSLock()
	LSSRLock()
	LSSRUnlock()
	LSSUnlock()
}

// ShardFn - shard function.
type ShardFn func(shard Shard) error

var ErrAlreadyDiscarded = errors.New("Head is already discarded")

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
	CreateDataStorageLoadAndQueryTask(shardID uint16, querier uintptr) *GenericTask
	UnloadUnusedSeriesData()
	Raw() Head
	UnrecoverableError(error)
}

type Distributor interface {
	Send(ctx context.Context, head Head, shardedData [][]*cppbridge.InnerSeries) error
	// DestinationGroups - workaround.
	DestinationGroups() DestinationGroups
	// SetDestinationGroups - workaround.
	SetDestinationGroups(destinationGroups DestinationGroups)
	WriteMetrics(head Head)
	Rotate() error
}

type HeadConfigurator interface {
	Configure(head Head) error
}

type DistributorConfigurator interface {
	Configure(distributor Distributor) error
}

type DestructibleIncomingData struct {
	data          *IncomingData
	destructCount atomic.Int64
}

func NewDestructibleIncomingData(data *IncomingData, destructCount int) *DestructibleIncomingData {
	d := &DestructibleIncomingData{
		data: data,
	}
	d.destructCount.Store(int64(destructCount))
	return d
}

func (d *DestructibleIncomingData) Data() *IncomingData {
	return d.data
}

func (d *DestructibleIncomingData) Destroy() {
	if d.destructCount.Add(-1) != 0 {
		return
	}
	d.data.Destroy()
}

// HeadStatus holds information about all shards.
type HeadStatus struct {
	HeadStats                   HeadStats  `json:"headStats"`
	SeriesCountByMetricName     []HeadStat `json:"seriesCountByMetricName"`
	LabelValueCountByLabelName  []HeadStat `json:"labelValueCountByLabelName"`
	MemoryInBytesByLabelName    []HeadStat `json:"memoryInBytesByLabelName"`
	SeriesCountByLabelValuePair []HeadStat `json:"seriesCountByLabelValuePair"`
}

// HeadStat holds the information about individual cardinality.
type HeadStat struct {
	Name  string `json:"name"`
	Value uint64 `json:"value"`
}

// HeadStats has information about the head.
type HeadStats struct {
	NumSeries     uint64 `json:"numSeries"`
	NumLabelPairs int    `json:"numLabelPairs"`
	ChunkCount    int64  `json:"chunkCount"`
	MinTime       int64  `json:"minTime"`
	MaxTime       int64  `json:"maxTime"`
}
