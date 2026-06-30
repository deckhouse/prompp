package poolprovider

import (
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/pool"
	"github.com/prometheus/prometheus/util/zeropool"
)

//
// Shard
//

// Shard the minimum required head Shard implementation.
type Shard interface {
	// ShardID returns the shard ID.
	ShardID() uint16
}

//
// HeadPool
//

// HeadPool pools for reusable objects.
type HeadPool[TGShard Shard] struct {
	// used to reuse tasks
	taskPool sync.Pool
	// use in appender
	shardedStateUpdatesPool    sync.Pool
	shardedRelabeledSeriesPool sync.Pool
	shardedInnerSeriesPool     sync.Pool
	statsPool                  zeropool.Pool[[]cppbridge.RelabelerStats]
	// use in querier
	snapshotsPool         zeropool.Pool[[]*cppbridge.LabelSetSnapshot]
	lssQueryResultsPool   zeropool.Pool[[]*cppbridge.LSSQueryResult]
	selectorsPool         zeropool.Pool[[]uintptr]
	seriesSetPool         zeropool.Pool[[]storage.SeriesSet]
	chunkSeriesSetPool    zeropool.Pool[[]storage.ChunkSeriesSet]
	serializedDataPool    zeropool.Pool[[]*cppbridge.DataStorageSerializedData]
	errorsPool            zeropool.Pool[[]error]
	sliceOfTimestampsPool zeropool.Pool[[][]int64]
	timestampsPool        pool.SlicePool[int64]
	seriesGroupsPool      zeropool.Pool[[]*cppbridge.SeriesGroups]
	nameIDsPool           pool.SlicePool[uint32]
}

// NewHeadPool init new [HeadPool], pools for reusable objects.
//
//revive:disable-next-line:function-length // this is constructor.
func NewHeadPool[TGShard Shard](numberOfShards uint16) *HeadPool[TGShard] {
	return &HeadPool[TGShard]{
		// used to reuse tasks
		taskPool: sync.Pool{
			New: func() any {
				return task.NewGenericEmpty[TGShard](numberOfShards)
			},
		},
		// use in appender
		shardedInnerSeriesPool: sync.Pool{
			New: func() any {
				return cppbridge.NewShardedInnerSeries(numberOfShards)
			},
		},
		shardedRelabeledSeriesPool: sync.Pool{
			New: func() any {
				return cppbridge.NewShardedRelabeledSeries(numberOfShards)
			},
		},
		shardedStateUpdatesPool: sync.Pool{
			New: func() any {
				return cppbridge.NewShardedStateUpdates(numberOfShards)
			},
		},
		statsPool: zeropool.New(func() []cppbridge.RelabelerStats {
			return make([]cppbridge.RelabelerStats, numberOfShards)
		}),
		// use in querier
		snapshotsPool: zeropool.New(func() []*cppbridge.LabelSetSnapshot {
			return make([]*cppbridge.LabelSetSnapshot, numberOfShards)
		}),
		lssQueryResultsPool: zeropool.New(func() []*cppbridge.LSSQueryResult {
			return make([]*cppbridge.LSSQueryResult, numberOfShards)
		}),
		selectorsPool: zeropool.New(func() []uintptr {
			return make([]uintptr, numberOfShards)
		}),
		seriesSetPool: zeropool.New(func() []storage.SeriesSet {
			return make([]storage.SeriesSet, numberOfShards)
		}),
		chunkSeriesSetPool: zeropool.New(func() []storage.ChunkSeriesSet {
			return make([]storage.ChunkSeriesSet, numberOfShards)
		}),
		serializedDataPool: zeropool.New(func() []*cppbridge.DataStorageSerializedData {
			return make([]*cppbridge.DataStorageSerializedData, numberOfShards)
		}),
		errorsPool: zeropool.New(func() []error {
			return make([]error, numberOfShards)
		}),
		sliceOfTimestampsPool: zeropool.New(func() [][]int64 {
			return make([][]int64, numberOfShards)
		}),
		timestampsPool: pool.NewSlicePool[int64]([]int{2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}),
		seriesGroupsPool: zeropool.New(func() []*cppbridge.SeriesGroups {
			return make([]*cppbridge.SeriesGroups, numberOfShards)
		}),
		nameIDsPool: pool.NewSlicePool[uint32]([]int{0, 1, 2, 3, 5}),
	}
}

// GetTask gets a [task.Generic] from the pool.
func (hp *HeadPool[TGShard]) GetTask() *task.Generic[TGShard] {
	return hp.taskPool.Get().(*task.Generic[TGShard])
}

// PutTask adds [task.Generic] to the pool.
func (hp *HeadPool[TGShard]) PutTask(t *task.Generic[TGShard]) {
	hp.taskPool.Put(t)
}

// GetShardedInnerSeries gets a [cppbridge.ShardedInnerSeries] from the pool.
func (hp *HeadPool[TGShard]) GetShardedInnerSeries() *cppbridge.ShardedInnerSeries {
	return hp.shardedInnerSeriesPool.Get().(*cppbridge.ShardedInnerSeries)
}

// PutShardedInnerSeries adds [cppbridge.ShardedInnerSeries] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutShardedInnerSeries(s *cppbridge.ShardedInnerSeries) {
	s.Reset()
	hp.shardedInnerSeriesPool.Put(s)
}

// GetShardedRelabeledSeries gets a [cppbridge.ShardedRelabeledSeries] from the pool.
func (hp *HeadPool[TGShard]) GetShardedRelabeledSeries() *cppbridge.ShardedRelabeledSeries {
	return hp.shardedRelabeledSeriesPool.Get().(*cppbridge.ShardedRelabeledSeries)
}

// PutShardedRelabeledSeries adds [cppbridge.ShardedRelabeledSeries] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutShardedRelabeledSeries(s *cppbridge.ShardedRelabeledSeries) {
	s.Reset()
	hp.shardedRelabeledSeriesPool.Put(s)
}

// GetShardedStateUpdates gets a [cppbridge.ShardedStateUpdates] from the pool.
func (hp *HeadPool[TGShard]) GetShardedStateUpdates() *cppbridge.ShardedStateUpdates {
	return hp.shardedStateUpdatesPool.Get().(*cppbridge.ShardedStateUpdates)
}

// PutShardedStateUpdates adds [cppbridge.ShardedStateUpdates] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutShardedStateUpdates(s *cppbridge.ShardedStateUpdates) {
	s.Reset()
	hp.shardedStateUpdatesPool.Put(s)
}

// GetRelabelerStats gets a slice of [cppbridge.RelabelerStats] from the pool.
func (hp *HeadPool[TGShard]) GetRelabelerStats() []cppbridge.RelabelerStats {
	return hp.statsPool.Get()
}

// PutRelabelerStats adds slice of [cppbridge.RelabelerStats] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutRelabelerStats(stats []cppbridge.RelabelerStats) {
	clear(stats)
	hp.statsPool.Put(stats)
}

// GetSnapshots gets a slice of [cppbridge.LabelSetSnapshot] from the pool.
func (hp *HeadPool[TGShard]) GetSnapshots() []*cppbridge.LabelSetSnapshot {
	return hp.snapshotsPool.Get()
}

// PutSnapshots adds slice of [cppbridge.LabelSetSnapshot] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutSnapshots(snapshots []*cppbridge.LabelSetSnapshot) {
	clear(snapshots)
	hp.snapshotsPool.Put(snapshots)
}

// GetLSSQueryResults gets a slice of [cppbridge.LSSQueryResult] from the pool.
func (hp *HeadPool[TGShard]) GetLSSQueryResults() []*cppbridge.LSSQueryResult {
	return hp.lssQueryResultsPool.Get()
}

// PutLSSQueryResults adds slice of [cppbridge.LSSQueryResult] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutLSSQueryResults(results []*cppbridge.LSSQueryResult) {
	clear(results)
	hp.lssQueryResultsPool.Put(results)
}

// GetSelectors gets a slice of [uintptr] from the pool.
func (hp *HeadPool[TGShard]) GetSelectors() []uintptr {
	return hp.selectorsPool.Get()
}

// PutSelectors adds slice of [uintptr] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutSelectors(selectors []uintptr) {
	clear(selectors)
	hp.selectorsPool.Put(selectors)
}

// GetSeriesSet gets a slice of [storage.SeriesSet] from the pool.
func (hp *HeadPool[TGShard]) GetSeriesSet() []storage.SeriesSet {
	return hp.seriesSetPool.Get()
}

// PutSeriesSet adds slice of [storage.SeriesSet] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutSeriesSet(ssets []storage.SeriesSet) {
	clear(ssets)
	hp.seriesSetPool.Put(ssets)
}

// GetChunkSeriesSet gets a slice of [storage.ChunkSeriesSet] from the pool.
func (hp *HeadPool[TGShard]) GetChunkSeriesSet() []storage.ChunkSeriesSet {
	return hp.chunkSeriesSetPool.Get()
}

// PutChunkSeriesSet adds slice of [storage.ChunkSeriesSet] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutChunkSeriesSet(csets []storage.ChunkSeriesSet) {
	clear(csets)
	hp.chunkSeriesSetPool.Put(csets)
}

// GetSerializedData gets a slice of [cppbridge.DataStorageSerializedData] from the pool.
func (hp *HeadPool[TGShard]) GetSerializedData() []*cppbridge.DataStorageSerializedData {
	return hp.serializedDataPool.Get()
}

// PutSerializedData adds slice of [cppbridge.DataStorageSerializedData] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutSerializedData(sd []*cppbridge.DataStorageSerializedData) {
	clear(sd)
	hp.serializedDataPool.Put(sd)
}

// GetErrors gets a slice of [error] from the pool.
func (hp *HeadPool[TGShard]) GetErrors() []error {
	return hp.errorsPool.Get()
}

// PutErrors adds slice of [error] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutErrors(errs []error) {
	clear(errs)
	hp.errorsPool.Put(errs)
}

// GetSliceOfTimestamps gets a slice of []int64 from the pool.
func (hp *HeadPool[TGShard]) GetSliceOfTimestamps() [][]int64 {
	return hp.sliceOfTimestampsPool.Get()
}

// PutSliceOfTimestamps adds slice of []int64 to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutSliceOfTimestamps(ts [][]int64) {
	clear(ts)
	hp.sliceOfTimestampsPool.Put(ts)
}

// GetTimestamps gets a slice of [int64] from the pool.
func (hp *HeadPool[TGShard]) GetTimestamps(size int) []int64 {
	return hp.timestampsPool.Get(size)
}

// PutTimestamps adds slice of [int64] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutTimestamps(ts []int64) {
	clear(ts)
	hp.timestampsPool.Put(ts)
}

// GetSeriesGroups gets a slice of [cppbridge.SeriesGroups] from the pool.
func (hp *HeadPool[TGShard]) GetSeriesGroups() []*cppbridge.SeriesGroups {
	return hp.seriesGroupsPool.Get()
}

// PutSeriesGroups adds slice of [cppbridge.SeriesGroups] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutSeriesGroups(groups []*cppbridge.SeriesGroups) {
	clear(groups)
	hp.seriesGroupsPool.Put(groups)
}

// GetNameIDs gets a slice of [uint32] from the pool.
func (hp *HeadPool[TGShard]) GetNameIDs(size int) []uint32 {
	return hp.nameIDsPool.Get(size)
}

// PutNameIDs adds slice of [uint32] to the pool after resetting it.
func (hp *HeadPool[TGShard]) PutNameIDs(nameIDs []uint32) {
	clear(nameIDs)
	hp.nameIDsPool.Put(nameIDs)
}
