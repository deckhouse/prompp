package head

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/util"
)

//
// GenericTask
//

// GenericTask generic task, will be executed on each shard.
type GenericTask struct {
	errs    []error
	shardFn relabeler.ShardFn
	wg      sync.WaitGroup
	created prometheus.Counter
	done    prometheus.Counter
	execute *prometheus.CounterVec
}

// NewGenericTask init new GenericTask.
func NewGenericTask(
	shardFn relabeler.ShardFn,
	registerer prometheus.Registerer,
	numberOfShards uint16,
	typeTask relabeler.TypeTask,
) *GenericTask {
	factory := util.NewUnconflictRegisterer(registerer)
	constLabels := prometheus.Labels{"type_task": typeTask.String()}
	t := &GenericTask{
		errs:    make([]error, numberOfShards),
		shardFn: shardFn,
		wg:      sync.WaitGroup{},
		created: factory.NewCounter(prometheus.CounterOpts{
			Name:        "prompp_head_task_created_count",
			Help:        "Number of created tasks.",
			ConstLabels: constLabels,
		}),
		done: factory.NewCounter(prometheus.CounterOpts{
			Name:        "prompp_head_task_done_count",
			Help:        "Number of done tasks.",
			ConstLabels: constLabels,
		}),
		execute: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "prompp_head_task_execute_duration_microseconds_sum",
				Help:        "The duration of the task execution in microseconds.",
				ConstLabels: constLabels,
			},
			[]string{"shard_id"},
		),
	}
	t.wg.Add(int(numberOfShards))
	t.created.Inc()

	return t
}

// NewSingleGenericTask init new GenericTask for single shard.
func NewSingleGenericTask(
	shardFn relabeler.ShardFn,
	registerer prometheus.Registerer,
	numberOfShards uint16,
	typeTask relabeler.TypeTask,
) *GenericTask {
	factory := util.NewUnconflictRegisterer(registerer)
	constLabels := prometheus.Labels{"type_task": typeTask.String()}
	t := &GenericTask{
		errs:    make([]error, numberOfShards),
		wg:      sync.WaitGroup{},
		shardFn: shardFn,
		created: factory.NewCounter(prometheus.CounterOpts{
			Name:        "prompp_head_task_created_count",
			Help:        "Number of created tasks.",
			ConstLabels: constLabels,
		}),
		done: factory.NewCounter(prometheus.CounterOpts{
			Name:        "prompp_head_task_done_count",
			Help:        "Number of done tasks.",
			ConstLabels: constLabels,
		}),
		execute: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "prompp_head_task_execute_duration_microseconds_sum",
				Help:        "The duration of the task execution in microseconds.",
				ConstLabels: constLabels,
			},
			[]string{"shard_id"},
		),
	}
	t.wg.Add(1)
	t.created.Inc()

	return t
}

// ExecuteOnShard execute task on shard.
func (t *GenericTask) ExecuteOnShard(shard relabeler.Shard) {
	start := time.Now()

	t.errs[shard.ShardID()] = t.shardFn(shard)
	t.wg.Done()

	t.execute.With(prometheus.Labels{
		"shard_id": strconv.FormatUint(uint64(shard.ShardID()), 10),
	}).Add(float64(time.Since(start).Microseconds()))
}

// Wait for the task to complete on all shards.
func (t *GenericTask) Wait() error {
	t.wg.Wait()
	t.done.Inc()

	return errors.Join(t.errs...)
}
