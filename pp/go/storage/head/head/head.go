package head

import (
	"context"
	"fmt"
	"sync"

	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/util/locker"
)

// ExtraWorkers number of extra workers for operation on shards.
var ExtraConcurrency = 0

//
// Shard
//

// Shard the minimum required head Shard implementation.
type Shard interface {
	// ShardID returns the shard ID.
	ShardID() uint16
}

//
// Head
//

type Head[TShard Shard] struct {
	id         string
	generation uint64
	readOnly   bool

	shards             []TShard
	lssTaskChs         []chan *task.Generic[TShard]
	dataStorageTaskChs []chan *task.Generic[TShard]
	querysemaphore     *locker.Weighted

	numberOfShards uint16
	stopc          chan struct{}
	wg             sync.WaitGroup

	// // stat
	// appendedSegmentCount prometheus.Counter
	// memoryInUse          *prometheus.GaugeVec
	// series               prometheus.Gauge
	// walSize              *prometheus.GaugeVec
	// // TODO refactoring
	// queueLSS         *prometheus.GaugeVec
	// queueDataStorage *prometheus.GaugeVec

	// tasksCreated *prometheus.CounterVec
	// tasksDone    *prometheus.CounterVec
	// tasksLive    *prometheus.CounterVec
	// tasksExecute *prometheus.CounterVec
}

// NewHead init new [Head].
func NewHead[TShard Shard](shards []TShard) *Head[TShard] {
	return &Head[TShard]{
		shards: shards,
	}
}

// AcquireQuery acquires the [Head] semaphore with a weight of 1,
// blocking until resources are available or ctx is done.
// On success, returns nil. On failure, returns ctx.Err() and leaves the semaphore unchanged.
func (h *Head[TShard]) AcquireQuery(ctx context.Context) (release func(), err error) {
	return h.querysemaphore.RLock(ctx)
}

// Concurrency return current head workers concurrency.
func (h *Head[TShard]) Concurrency() int64 {
	return calculateHeadConcurrency(h.numberOfShards)
}

// CreateTask create a task for operations on the [Head] shards.
func (h *Head[TShard]) CreateTask(taskName string, fn func(shard TShard) error) *task.Generic[TShard] {
	// TODO
	return nil
}

// Enqueue the task to be executed on shards [Head].
func (h *Head[TShard]) Enqueue(t *task.Generic[TShard]) {
	// TODO
}

// ID returns id [Head].
func (h *Head[TShard]) ID() string {
	return h.id
}

// NumberOfShards returns current number of shards in to [Head].
func (h *Head[TShard]) NumberOfShards() uint16 {
	return h.numberOfShards
}

// String serialize as string.
func (h *Head[TShard]) String() string {
	return fmt.Sprintf("{id: %s, generation: %d}", h.id, h.generation)
}

// calculateHeadConcurrency calculate current head workers concurrency.
func calculateHeadConcurrency(numberOfShards uint16) int64 {
	//revive:disable-next-line:add-constant 2 - default run workers
	return 2 * int64(1+ExtraConcurrency) * int64(numberOfShards)
}
