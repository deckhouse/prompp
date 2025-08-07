package head

import (
	"sync"

	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/util/locker"
)

type Shard interface {
	ShardID() uint16
}

type Head struct {
	id         string
	generation uint64
	readOnly   bool

	shards             []Shard
	lssTaskChs         []chan *relabeler.GenericTask
	dataStorageTaskChs []chan *relabeler.GenericTask
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
