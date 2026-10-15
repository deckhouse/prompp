package querier

import (
	"sync"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
)

const (
	// dsLoadAndQuerySeriesData.
	dsLoadAndQuerySeriesData = "data_storage_load_and_query_series_data"
)

// LoadAndQueryWaiter is a waiter for the load and query series data task.
type LoadAndQueryWaiter[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
] struct {
	waiter     task.Waiter[shard.Task]
	head       THead
	locker     sync.Mutex
	hasQueries bool
}

// NewLoadAndQueryWaiter creates a new [LoadAndQueryWaiter].
func NewLoadAndQueryWaiter[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
](head THead) LoadAndQueryWaiter[TTask, TDataStorage, TLSS, TShard, THead] {
	return LoadAndQueryWaiter[TTask, TDataStorage, TLSS, TShard, THead]{
		head:   head,
		locker: sync.Mutex{},
	}
}

// Add adds a querier to the load and query series data task.
func (l *LoadAndQueryWaiter[TTask, TDataStorage, TLSS, TShard, THead]) Add(s TShard, querier uintptr) {
	l.locker.Lock()
	l.hasQueries = true
	l.waiter.Add(s.LoadAndQuerySeriesDataTask().Add(querier, func() shard.Task {
		t := l.head.CreateTask(dsLoadAndQuerySeriesData, func(s TShard) error {
			return s.LoadAndQuerySeriesData()
		})
		l.head.EnqueueOnShard(t, s.ShardID())
		return t
	}))
	l.locker.Unlock()
}

// HasQueries reports whether Add was called, even if those queries have already completed.
// Call only after waiting for the primary task, so that all concurrent Add calls have finished.
func (l *LoadAndQueryWaiter[TTask, TDataStorage, TLSS, TShard, THead]) HasQueries() bool {
	return l.hasQueries
}

// Wait waits for the load and query series data task to complete.
func (l *LoadAndQueryWaiter[TTask, TDataStorage, TLSS, TShard, THead]) Wait() error {
	return l.waiter.Wait()
}
