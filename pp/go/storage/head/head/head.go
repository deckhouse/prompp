package head

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/pp/go/util/locker"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/zeropool"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg head_test --out
//go:generate moq head_moq_test.go . Shard

// ExtraWorkers number of extra workers for operation on shards.
var ExtraWorkers = 1

// defaultNumberOfWorkers default number of workers.
const defaultNumberOfWorkers = 2

//
// Shard
//

// Shard the minimum required head Shard implementation.
type Shard interface {
	// ShardID returns the shard ID.
	ShardID() uint16

	// Close closes the wal segmentWriter.
	Close() error
}

//
// Head
//

// Head stores and manages shards, handles reads and writes of time series data within a time window.
type Head[TShard Shard, TGorutineShard Shard] struct {
	id         string
	generation uint64

	gshardCtor    func(s TShard, numberOfShards uint16) TGorutineShard
	releaseHeadFn func()

	shards         []TShard
	taskChs        []chan *task.Generic[TGorutineShard]
	querySemaphore *locker.Weighted

	stopc     chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once

	readOnly uint32

	// for clearing [Head] metrics
	memoryInUse *prometheus.GaugeVec
	// for tasks metrics
	tasksCreated *prometheus.CounterVec
	tasksDone    *prometheus.CounterVec
	tasksLive    *prometheus.CounterVec
	tasksExecute *prometheus.CounterVec

	// pools for reusable objects
	// use in appender
	shardedInnerSeriesPool     sync.Pool
	shardedRelabeledSeriesPool sync.Pool
	shardedStateUpdatesPool    sync.Pool
	taskPool                   sync.Pool
	statsPool                  zeropool.Pool[[]cppbridge.RelabelerStats]
	// use in querier
	snapshotsPool       zeropool.Pool[[]*cppbridge.LabelSetSnapshot]
	lssQueryResultsPool zeropool.Pool[[]*cppbridge.LSSQueryResult]
	selectorsPool       zeropool.Pool[[]uintptr]
	seriesSetPool       zeropool.Pool[[]storage.SeriesSet]
	chunkSeriesSetPool  zeropool.Pool[[]storage.ChunkSeriesSet]
	serializedDataPool  zeropool.Pool[[]*cppbridge.DataStorageSerializedData]
	errorsPool          zeropool.Pool[[]error]
}

// NewHead init new [Head].
//
//revive:disable-next-line:function-length long but readable.
func NewHead[TShard Shard, TGoroutineShard Shard](
	id string,
	shards []TShard,
	gshardCtor func(TShard, uint16) TGoroutineShard,
	releaseHeadFn func(),
	generation uint64,
	registerer prometheus.Registerer,
) *Head[TShard, TGoroutineShard] {
	numberOfShards := len(shards)
	taskChs := make([]chan *task.Generic[TGoroutineShard], numberOfShards)
	concurrency := calculateHeadConcurrency(numberOfShards) // current head workers concurrency
	for shardID := range numberOfShards {
		// append and query can create 2 tasks per request, so minimal length of channel is
		// cap(querySemaphore)*2+cap(appendSemaphore)*2 = 2*concurrency*2+2*concurrency*2 = 8*concurrency
		// add extra slots to channel for safety = x9 for back pressure
		taskChs[shardID] = make(chan *task.Generic[TGoroutineShard], 9*concurrency)
	}

	factory := util.NewUnconflictRegisterer(registerer)
	numShards := uint16(numberOfShards) // #nosec G115 // no overflow
	h := &Head[TShard, TGoroutineShard]{
		id:             id,
		generation:     generation,
		gshardCtor:     gshardCtor,
		releaseHeadFn:  releaseHeadFn,
		shards:         shards,
		taskChs:        taskChs,
		querySemaphore: locker.NewWeighted(2 * concurrency), // x2 for back pressure
		stopc:          make(chan struct{}),
		wg:             sync.WaitGroup{},
		closeOnce:      sync.Once{},

		// for clearing [Head] metrics
		memoryInUse: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prompp_head_cgo_memory_bytes",
			Help: "Current value memory in use in bytes.",
		}, []string{"head_id", "allocator", "shard_id"}),
		// for tasks metrics
		tasksCreated: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "prompp_head_task_created_count",
			Help: "Number of created tasks.",
		}, []string{"type_task"}),
		tasksDone: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "prompp_head_task_done_count",
			Help: "Number of done tasks.",
		}, []string{"type_task"}),

		// pools for reusable objects
		shardedInnerSeriesPool: sync.Pool{
			New: func() any {
				return cppbridge.NewShardedInnerSeries(numShards)
			},
		},
		shardedRelabeledSeriesPool: sync.Pool{
			New: func() any {
				return cppbridge.NewShardedRelabeledSeries(numShards)
			},
		},
		shardedStateUpdatesPool: sync.Pool{
			New: func() any {
				return cppbridge.NewShardedStateUpdates(numShards)
			},
		},
		taskPool: sync.Pool{
			New: func() any {
				return task.NewGenericEmpty[TGoroutineShard](numShards)
			},
		},
		statsPool: zeropool.New(func() []cppbridge.RelabelerStats {
			return make([]cppbridge.RelabelerStats, numberOfShards)
		}),
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
	}

	h.run()

	runtime.SetFinalizer(h, func(h *Head[TShard, TGoroutineShard]) {
		h.memoryInUse.DeletePartialMatch(prometheus.Labels{"head_id": h.id})
		logger.Debugf("[Head] %s destroyed", h.String())
	})

	logger.Debugf("[Head] %s created", h.String())

	return h
}

// AcquireQuery acquires the [Head] semaphore with a weight of 1,
// blocking until resources are available or ctx is done.
// On success, returns nil. On failure, returns ctx.Err() and leaves the semaphore unchanged.
func (h *Head[TShard, TGorutineShard]) AcquireQuery(ctx context.Context) (release func(), err error) {
	return h.querySemaphore.RLock(ctx)
}

// Close closes wals, query semaphore for the inability to get query and clear metrics.
func (h *Head[TShard, TGorutineShard]) Close() (err error) {
	h.closeOnce.Do(func() {
		if err = h.querySemaphore.Close(); err != nil {
			return
		}

		close(h.stopc)
		h.wg.Wait()

		for _, s := range h.shards {
			err = errors.Join(err, s.Close())
		}

		if h.releaseHeadFn != nil {
			h.releaseHeadFn()
		}

		logger.Debugf("[Head] %s is closed", h.String())
	})

	return err
}

// Concurrency return current head workers concurrency.
func (h *Head[TShard, TGorutineShard]) Concurrency() int64 {
	return calculateHeadConcurrency(len(h.shards))
}

// CreateTask create a task for operations on the [Head] shards.
func (h *Head[TShard, TGorutineShard]) CreateTask(
	taskName string,
	shardFn func(shard TGorutineShard) error,
) *task.Generic[TGorutineShard] {
	t := h.taskPool.Get().(*task.Generic[TGorutineShard])
	t.Reset(
		shardFn,
		h.tasksDone.WithLabelValues(taskName),
	)
	h.tasksCreated.WithLabelValues(taskName).Inc()
	return t
}

// ReleaseTask returns a task to the pool.
func (h *Head[TShard, TGorutineShard]) ReleaseTask(t *task.Generic[TGorutineShard]) {
	h.taskPool.Put(t)
}

// Enqueue the task to be executed on shards [Head].
func (h *Head[TShard, TGorutineShard]) Enqueue(t *task.Generic[TGorutineShard]) {
	t.SetShardsNumber(h.NumberOfShards())

	for _, taskCh := range h.taskChs {
		taskCh <- t
	}
}

// EnqueueOnShard the task to be executed on head on specific shard.
func (h *Head[TShard, TGorutineShard]) EnqueueOnShard(t *task.Generic[TGorutineShard], shardID uint16) {
	t.SetShardsNumber(1)

	h.taskChs[shardID] <- t
}

// Generation returns current generation of [Head].
func (h *Head[TShard, TGorutineShard]) Generation() uint64 {
	return h.generation
}

// ID returns id [Head].
func (h *Head[TShard, TGorutineShard]) ID() string {
	return h.id
}

// IsReadOnly returns true if the [Head] has switched to read-only.
func (h *Head[TShard, TGorutineShard]) IsReadOnly() bool {
	return atomic.LoadUint32(&h.readOnly) > 0
}

// NumberOfShards returns current number of shards in to [Head].
func (h *Head[TShard, TGorutineShard]) NumberOfShards() uint16 {
	return uint16(len(h.shards)) // #nosec G115 // no overflow
}

// RangeQueueSize returns an iterator over the [Head] task channels, to collect metrics.
func (h *Head[TShard, TGorutineShard]) RangeQueueSize() func(func(shardID, size int) bool) {
	return func(yield func(shardID, size int) bool) {
		for shardID, taskCh := range h.taskChs {
			if !yield(shardID, len(taskCh)) {
				return
			}
		}
	}
}

// RangeShards returns an iterator over the [Head] [Shard]s, through which the shard can be directly accessed.
func (h *Head[TShard, TGorutineShard]) RangeShards() func(func(TShard) bool) {
	return func(yield func(s TShard) bool) {
		for _, shard := range h.shards {
			if !yield(shard) {
				return
			}
		}
	}
}

// AcquireShardedInnerSeries gets a [cppbridge.ShardedInnerSeries] from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireShardedInnerSeries() *cppbridge.ShardedInnerSeries {
	return h.shardedInnerSeriesPool.Get().(*cppbridge.ShardedInnerSeries)
}

// ReleaseShardedInnerSeries returns a [cppbridge.ShardedInnerSeries] to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseShardedInnerSeries(s *cppbridge.ShardedInnerSeries) {
	s.Reset()
	h.shardedInnerSeriesPool.Put(s)
}

// AcquireShardedRelabeledSeries gets a [cppbridge.ShardedRelabeledSeries] from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireShardedRelabeledSeries() *cppbridge.ShardedRelabeledSeries {
	return h.shardedRelabeledSeriesPool.Get().(*cppbridge.ShardedRelabeledSeries)
}

// ReleaseShardedRelabeledSeries returns a [cppbridge.ShardedRelabeledSeries] to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseShardedRelabeledSeries(s *cppbridge.ShardedRelabeledSeries) {
	s.Reset()
	h.shardedRelabeledSeriesPool.Put(s)
}

// AcquireShardedStateUpdates gets a [cppbridge.ShardedStateUpdates] from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireShardedStateUpdates() *cppbridge.ShardedStateUpdates {
	return h.shardedStateUpdatesPool.Get().(*cppbridge.ShardedStateUpdates)
}

// ReleaseShardedStateUpdates returns a [cppbridge.ShardedStateUpdates] to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseShardedStateUpdates(s *cppbridge.ShardedStateUpdates) {
	s.Reset()
	h.shardedStateUpdatesPool.Put(s)
}

// AcquireRelabelerStats gets a []cppbridge.RelabelerStats from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireRelabelerStats() []cppbridge.RelabelerStats {
	return h.statsPool.Get()
}

// ReleaseRelabelerStats returns a []cppbridge.RelabelerStats to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseRelabelerStats(stats []cppbridge.RelabelerStats) {
	clear(stats)
	h.statsPool.Put(stats)
}

// AcquireSnapshots gets a []*cppbridge.LabelSetSnapshot from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireSnapshots() []*cppbridge.LabelSetSnapshot {
	return h.snapshotsPool.Get()
}

// ReleaseSnapshots returns a []*cppbridge.LabelSetSnapshot to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseSnapshots(snapshots []*cppbridge.LabelSetSnapshot) {
	clear(snapshots)
	h.snapshotsPool.Put(snapshots)
}

// AcquireLSSQueryResults gets a []*cppbridge.LSSQueryResult from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireLSSQueryResults() []*cppbridge.LSSQueryResult {
	return h.lssQueryResultsPool.Get()
}

// ReleaseLSSQueryResults returns a []*cppbridge.LSSQueryResult to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseLSSQueryResults(results []*cppbridge.LSSQueryResult) {
	clear(results)
	h.lssQueryResultsPool.Put(results)
}

// AcquireSelectors gets a []uintptr from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireSelectors() []uintptr {
	return h.selectorsPool.Get()
}

// ReleaseSelectors returns a []uintptr to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseSelectors(selectors []uintptr) {
	clear(selectors)
	h.selectorsPool.Put(selectors)
}

// AcquireSeriesSet gets a []storage.SeriesSet from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireSeriesSet() []storage.SeriesSet {
	return h.seriesSetPool.Get()
}

// ReleaseSeriesSet returns a []storage.SeriesSet to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseSeriesSet(ssets []storage.SeriesSet) {
	clear(ssets)
	h.seriesSetPool.Put(ssets)
}

// AcquireChunkSeriesSet gets a []storage.ChunkSeriesSet from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireChunkSeriesSet() []storage.ChunkSeriesSet {
	return h.chunkSeriesSetPool.Get()
}

// ReleaseChunkSeriesSet returns a []storage.ChunkSeriesSet to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseChunkSeriesSet(csets []storage.ChunkSeriesSet) {
	clear(csets)
	h.chunkSeriesSetPool.Put(csets)
}

// AcquireSerializedData gets a []*cppbridge.DataStorageSerializedData from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireSerializedData() []*cppbridge.DataStorageSerializedData {
	return h.serializedDataPool.Get()
}

// ReleaseSerializedData returns a []*cppbridge.DataStorageSerializedData to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseSerializedData(sd []*cppbridge.DataStorageSerializedData) {
	clear(sd)
	h.serializedDataPool.Put(sd)
}

// AcquireErrors gets a []error from the pool.
func (h *Head[TShard, TGorutineShard]) AcquireErrors() []error {
	return h.errorsPool.Get()
}

// ReleaseErrors returns a []error to the pool after resetting it.
func (h *Head[TShard, TGorutineShard]) ReleaseErrors(errs []error) {
	clear(errs)
	h.errorsPool.Put(errs)
}

// SetReadOnly sets the read-only flag for the [Head].
func (h *Head[TShard, TGorutineShard]) SetReadOnly() {
	atomic.StoreUint32(&h.readOnly, 1)
}

// Shards returns the [Head] [Shard]s.
func (h *Head[TShard, TGoroutineShard]) Shards() []TShard {
	return h.shards
}

// String serialize as string.
func (h *Head[TShard, TGorutineShard]) String() string {
	return fmt.Sprintf("{id: %s, generation: %d}", h.id, h.generation)
}

// run loop for each shard.
func (h *Head[TShard, TGorutineShard]) run() {
	workers := defaultNumberOfWorkers + ExtraWorkers
	numberOfShards := len(h.shards)
	h.wg.Add(workers * numberOfShards)
	for shardID := range numberOfShards {
		for range workers {
			go func(sid int) {
				defer h.wg.Done()
				h.shardLoop(h.taskChs[sid], h.stopc, h.shards[sid])
			}(shardID)
		}
	}
}

// shardLoop run shard loop for operation.
func (h *Head[TShard, TGorutineShard]) shardLoop(
	taskCH chan *task.Generic[TGorutineShard],
	stopc chan struct{},
	s TShard,
) {
	pgs := h.gshardCtor(s, h.NumberOfShards())

	for {
		select {
		case <-stopc:
			return

		case t := <-taskCH:
			t.ExecuteOnShard(pgs)
		}
	}
}

// calculateHeadConcurrency calculate current head workers concurrency.
func calculateHeadConcurrency(numberOfShards int) int64 {
	return int64(defaultNumberOfWorkers+ExtraWorkers) * int64(numberOfShards)
}

//
// CopyAddedSeries
//

// CopyAddedSeries copy the label sets from the source lss to the destination lss that were added source lss.
func CopyAddedSeries[TShard Shard, TGorutineShard Shard](
	shardCopier func(source, destination TShard),
) func(source, destination *Head[TShard, TGorutineShard]) {
	return func(source, destination *Head[TShard, TGorutineShard]) {
		if source.NumberOfShards() != destination.NumberOfShards() {
			logger.Warnf(
				"source[%d] and destination[%d] number of shards must be the same",
				source.NumberOfShards(),
				destination.NumberOfShards(),
			)

			return
		}

		for shardID := range source.NumberOfShards() {
			shardCopier(source.shards[shardID], destination.shards[shardID])
		}
	}
}
