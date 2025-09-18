package shard

import (
	"sync"
)

// Task the minimum required Task implementation.
type Task interface {
	Wait() error
}

type LoadAndQuerySeriesDataTask struct {
	queriers []uintptr
	task     Task
	lock     sync.Mutex
}

func (t *LoadAndQuerySeriesDataTask) Add(querier uintptr, createAndEnqueueTask func() Task) Task {
	t.lock.Lock()
	t.queriers = append(t.queriers, querier)
	if len(t.queriers) == 1 {
		t.task = createAndEnqueueTask()
	}
	t.lock.Unlock()

	return t.task
}

func (t *LoadAndQuerySeriesDataTask) Release(callback func([]uintptr)) {
	t.lock.Lock()
	callback(t.queriers)
	t.queriers = nil
	t.task = nil
	t.lock.Unlock()
}
