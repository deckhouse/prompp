package shard

import (
	"sync"
)

// Task the minimum required Task implementation.
type Task interface {
	Wait() error
}

// LoadAndQuerySeriesDataTask represents a task to load and query series data.
type LoadAndQuerySeriesDataTask struct {
	queriers []uintptr
	task     Task
	lock     sync.Mutex
}

// Add adds a querier to the task, if exists no task, it creates and enqueues a task.
func (t *LoadAndQuerySeriesDataTask) Add(querier uintptr, createAndEnqueueTask func() Task) Task {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.queriers = append(t.queriers, querier)
	if len(t.queriers) == 1 {
		t.task = createAndEnqueueTask()
	}

	return t.task
}

// Release executes and releases the queriers.
func (t *LoadAndQuerySeriesDataTask) Release(callback func([]uintptr)) {
	t.lock.Lock()
	callback(t.queriers)
	t.queriers = nil
	t.task = nil
	t.lock.Unlock()
}
