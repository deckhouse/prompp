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
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/pp/go/util/locker"
)

// ExtraWorkers number of extra workers for operation on shards.
var ExtraWorkers = 0

// defaultNumberOfWorkers default number of workers.
const defaultNumberOfWorkers = 2

//
// Shard
//

// Shard the minimum required head Shard implementation.
type Shard interface {
	// LSS() *LSS

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
		taskChs[shardID] = make(chan *task.Generic[TGoroutineShard], 4*concurrency) // x4 for back pressure
	}

	factory := util.NewUnconflictRegisterer(registerer)
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
		tasksLive: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "prompp_head_task_live_duration_microseconds_sum",
			Help: "The duration of the live task in microseconds.",
		}, []string{"type_task"}),
		tasksExecute: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "prompp_head_task_execute_duration_microseconds_sum",
			Help: "The duration of the task execution in microseconds.",
		}, []string{"type_task"}),
	}

	h.run()

	runtime.SetFinalizer(h, func(h *Head[TShard, TGoroutineShard]) {
		h.memoryInUse.DeletePartialMatch(prometheus.Labels{"head_id": h.id})
		logger.Debugf("[Head] %s destroyed.", h.String())
	})

	logger.Debugf("[Head] %s created.", h.String())

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
	ls := prometheus.Labels{"type_task": taskName}

	return task.NewGeneric(
		shardFn,
		h.tasksCreated.With(ls),
		h.tasksDone.With(ls),
		h.tasksLive.With(ls),
		h.tasksExecute.With(ls),
	)
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

// SetReadOnly sets the read-only flag for the [Head].
func (h *Head[TShard, TGorutineShard]) SetReadOnly() {
	atomic.StoreUint32(&h.readOnly, 1)
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
