package head

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/head/poolprovider"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/pp/go/util/locker"
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
type Head[TShard Shard, TGShard Shard] struct {
	id         string
	generation uint64

	gshardCtor    func(s TShard, numberOfShards uint16) TGShard
	releaseHeadFn func()

	shards         []TShard
	taskChs        []chan *task.Generic[TGShard]
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
	headPool *poolprovider.HeadPool[TGShard]
}

// NewHead init new [Head].
//
//revive:disable-next-line:function-length long but readable.
func NewHead[TShard Shard, TGShard Shard](
	id string,
	shards []TShard,
	gshardCtor func(TShard, uint16) TGShard,
	releaseHeadFn func(),
	generation uint64,
	registerer prometheus.Registerer,
) *Head[TShard, TGShard] {
	numberOfShards := len(shards)
	taskChs := make([]chan *task.Generic[TGShard], numberOfShards)
	concurrency := calculateHeadConcurrency(numberOfShards) // current head workers concurrency
	for shardID := range numberOfShards {
		// append and query can create 2 tasks per request, so minimal length of channel is
		// cap(querySemaphore)*2+cap(appendSemaphore)*2 = 2*concurrency*2+2*concurrency*2 = 8*concurrency
		// add extra slots to channel for safety = x9 for back pressure
		taskChs[shardID] = make(chan *task.Generic[TGShard], 9*concurrency)
	}

	factory := util.NewUnconflictRegisterer(registerer)
	numShards := uint16(numberOfShards) // #nosec G115 // no overflow
	h := &Head[TShard, TGShard]{
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
		headPool: poolprovider.NewHeadPool[TGShard](numShards),
	}

	h.run()

	runtime.SetFinalizer(h, func(h *Head[TShard, TGShard]) {
		h.memoryInUse.DeletePartialMatch(prometheus.Labels{"head_id": h.id})
		logger.Debugf("[Head] %s destroyed", h.String())
	})

	logger.Debugf("[Head] %s created", h.String())

	return h
}

// AcquireQuery acquires the [Head] semaphore with a weight of 1,
// blocking until resources are available or ctx is done.
// On success, returns nil. On failure, returns ctx.Err() and leaves the semaphore unchanged.
func (h *Head[TShard, TGShard]) AcquireQuery(ctx context.Context) (release func(), err error) {
	return h.querySemaphore.RLock(ctx)
}

// Close closes wals, query semaphore for the inability to get query and clear metrics.
func (h *Head[TShard, TGShard]) Close() (err error) {
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
func (h *Head[TShard, TGShard]) Concurrency() int64 {
	return calculateHeadConcurrency(len(h.shards))
}

// CreateTask creates a [task.Generic] for operations on the [Head] shards.
func (h *Head[TShard, TGShard]) CreateTask(
	taskName string,
	shardFn func(shard TGShard) error,
) *task.Generic[TGShard] {
	t := h.headPool.GetTask()
	t.Reset(
		shardFn,
		h.tasksDone.WithLabelValues(taskName),
	)
	h.tasksCreated.WithLabelValues(taskName).Inc()
	return t
}

// Enqueue the task to be executed on shards [Head].
func (h *Head[TShard, TGShard]) Enqueue(t *task.Generic[TGShard]) {
	t.SetShardsNumber(h.NumberOfShards())

	for _, taskCh := range h.taskChs {
		taskCh <- t
	}
}

// EnqueueOnShard the task to be executed on head on specific shard.
func (h *Head[TShard, TGShard]) EnqueueOnShard(t *task.Generic[TGShard], shardID uint16) {
	t.SetShardsNumber(1)

	h.taskChs[shardID] <- t
}

// Generation returns current generation of [Head].
func (h *Head[TShard, TGShard]) Generation() uint64 {
	return h.generation
}

// ID returns id [Head].
func (h *Head[TShard, TGShard]) ID() string {
	return h.id
}

// IsReadOnly returns true if the [Head] has switched to read-only.
func (h *Head[TShard, TGShard]) IsReadOnly() bool {
	return atomic.LoadUint32(&h.readOnly) > 0
}

// NumberOfShards returns current number of shards in to [Head].
func (h *Head[TShard, TGShard]) NumberOfShards() uint16 {
	return uint16(len(h.shards)) // #nosec G115 // no overflow
}

// PoolProvider returns the [poolprovider.HeadPool] for the [Head].
func (h *Head[TShard, TGShard]) PoolProvider() *poolprovider.HeadPool[TGShard] {
	return h.headPool
}

// PutTask adds [task.Generic] to the pool.
func (h *Head[TShard, TGShard]) PutTask(t *task.Generic[TGShard]) {
	h.headPool.PutTask(t)
}

// RangeQueueSize returns an iterator over the [Head] task channels, to collect metrics.
func (h *Head[TShard, TGShard]) RangeQueueSize() func(func(shardID, size int) bool) {
	return func(yield func(shardID, size int) bool) {
		for shardID, taskCh := range h.taskChs {
			if !yield(shardID, len(taskCh)) {
				return
			}
		}
	}
}

// RangeShards returns an iterator over the [Head] [Shard]s, through which the shard can be directly accessed.
func (h *Head[TShard, TGShard]) RangeShards() func(func(TShard) bool) {
	return func(yield func(s TShard) bool) {
		for _, shard := range h.shards {
			if !yield(shard) {
				return
			}
		}
	}
}

// SetReadOnly sets the read-only flag for the [Head].
func (h *Head[TShard, TGShard]) SetReadOnly() {
	atomic.StoreUint32(&h.readOnly, 1)
}

// Shards returns the [Head] [Shard]s.
func (h *Head[TShard, TGShard]) Shards() []TShard {
	return h.shards
}

// String serialize as string.
func (h *Head[TShard, TGShard]) String() string {
	return fmt.Sprintf("{id: %s, generation: %d}", h.id, h.generation)
}

// run loop for each shard.
func (h *Head[TShard, TGShard]) run() {
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
func (h *Head[TShard, TGShard]) shardLoop(
	taskCH chan *task.Generic[TGShard],
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
func CopyAddedSeries[TShard Shard, TGShard Shard](
	shardCopier func(source, destination TShard),
) func(source, destination *Head[TShard, TGShard]) {
	return func(source, destination *Head[TShard, TGShard]) {
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
