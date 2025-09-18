package querier

import (
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
)

const (
	// dsLoadAndQuerySeriesData
	dsLoadAndQuerySeriesData = "data_storage_load_and_query_series_data"
)

type LoadAndQueryWaiter[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
] struct {
	waiter task.Waiter[shard.Task]
	head   THead
}

func NewLoadAndQueryWaiter[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
](head THead) LoadAndQueryWaiter[TTask, TDataStorage, TLSS, TShard, THead] {
	return LoadAndQueryWaiter[TTask, TDataStorage, TLSS, TShard, THead]{
		head: head,
	}
}

func (l *LoadAndQueryWaiter[TTask, TDataStorage, TLSS, TShard, THead]) Add(s TShard, querier uintptr) {
	l.waiter.Add(s.LoadAndQuerySeriesDataTask().Add(querier, func() shard.Task {
		t := l.head.CreateTask(dsLoadAndQuerySeriesData, func(s TShard) error {
			return s.LoadAndQuerySeriesData()
		})
		l.head.EnqueueOnShard(t, s.ShardID())
		return t
	}))
}

func (l *LoadAndQueryWaiter[TTask, TDataStorage, TLSS, TShard, THead]) Wait() error {
	return l.waiter.Wait()
}
