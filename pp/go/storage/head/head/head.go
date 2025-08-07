package head

import (
	"context"
	"sync"

	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/util/locker"
)

//
// Shard
//

// Shard the minimum required head Shard implementation.
type Shard interface {
	// QueryLabelValues(
	// 	name string,
	// 	matchers []model.LabelMatcher,
	// 	dedupAdd func(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string),
	// ) error
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
	queryLocker        *locker.Weighted

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

func NewHead[TShard Shard](shards []TShard) *Head[TShard] {
	return &Head[TShard]{
		shards: shards,
	}
}

// CreateTask create a task for operations on the head shards.
func (h *Head[TShard]) CreateTask(taskName string, fn func(shard TShard) error) *task.Generic[TShard] {
	// TODO
	return nil
}

// Enqueue the task to be executed on head.
func (h *Head[TShard]) Enqueue(t *task.Generic[TShard]) {
	// TODO
}

// NumberOfShards returns current number of shards.
func (h *Head[TShard]) NumberOfShards() uint16 {
	return h.numberOfShards
}

// RLockQuery locks for query to [Head].
func (h *Head[TShard]) RLockQuery(ctx context.Context) (runlock func(), err error) {
	// TODO
	return func() {}, nil
}
