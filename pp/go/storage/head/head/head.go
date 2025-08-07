package head

import (
	"context"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/util/locker"
)

type Shard interface {
	QueryLabelValues(
		name string,
		matchers []model.LabelMatcher,
		dedupAdd func(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string),
	) error
	ShardID() uint16
}

type Head struct {
	id         string
	generation uint64
	readOnly   bool

	shards             []Shard
	lssTaskChs         []chan *storage.GenericTask
	dataStorageTaskChs []chan *storage.GenericTask
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

func NewHead(shards []Shard) *Head {
	return &Head{
		shards: shards,
	}
}

// CreateTask create a task for operations on the head shards.
func (h *Head) CreateTask(taskName string, fn func(shard TShard) error, isLss bool) TGenericTask {
}

// Enqueue the task to be executed on head.
func (h *Head) Enqueue(t TGenericTask)

// NumberOfShards returns current number of shards.
func (h *Head) NumberOfShards() uint16 {
	return h.numberOfShards
}

// RLockQuery locks for query to [Head].
func (h *Head) RLockQuery(ctx context.Context) (runlock func(), err error)
